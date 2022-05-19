// Package archive extracts archives with a progress bar.
package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	bufra "github.com/avvmoto/buf-readerat"
	"github.com/blakesmith/ar"
	"github.com/gabriel-vasile/mimetype"
	"github.com/klauspost/compress/zstd"
	"github.com/saracen/go7z"
	"github.com/sassoftware/go-rpmutils"
	"github.com/xi2/xz"
	"howett.net/plist"

	"github.com/otiai10/copy"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/internal/system"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

// Extract from "source" to package destination.
//
// "finalise" must be called to complete extraction of the package.
func Extract(b *ui.Task, source string, pkg *manifest.Package) (finalise func() error, err error) {
	task := b.SubTask("unpack")
	finalise = func() error {
		return nil
	}
	if _, err := os.Stat(pkg.Dest); err == nil {
		return finalise, errors.Errorf("destination %s already exists", pkg.Dest)
	}
	task.Debugf("Extracting %s to %s", source, pkg.Dest)
	// Do we need to rename the result to the final pkg.Dest?
	// This is set to false if we are recursively extracting packages within one another
	renameResult := true

	isDir, err := isDirectory(source)
	if err != nil {
		return finalise, errors.WithStack(err)
	}
	if isDir {
		return finalise, installFromDirectory(source, pkg)
	}

	ext := filepath.Ext(source)
	switch ext {
	case ".pkg":
		return finalise, extractMacPKG(task, source, pkg.Dest, pkg.Strip)

	case ".dmg":
		return finalise, installMacDMG(task, source, pkg)
	}

	parentDir := filepath.Dir(pkg.Dest)
	if err := os.MkdirAll(parentDir, 0700); err != nil {
		return finalise, errors.WithStack(err)
	}

	tmpDest, err := ioutil.TempDir(parentDir, filepath.Base(pkg.Dest)+"-*")
	if err != nil {
		return finalise, errors.WithStack(err)
	}

	// Make the unpacked destination files read-only.
	if !pkg.Mutable {
		finalise = func() error {
			return errors.WithStack(filepath.Walk(pkg.Dest, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				task.Tracef("chmod a-w %q", path)
				err = os.Chmod(path, info.Mode()&^0222)
				if errors.Is(err, os.ErrNotExist) {
					task.Debugf("file did not exist during finalisation %q", path)
					return nil
				}
				return errors.WithStack(err)
			}))
		}
	}

	// Cleanup or finalise temporary directory.
	defer func() {
		if err != nil {
			task.Tracef("rm -rf %q", tmpDest)
			_ = os.RemoveAll(tmpDest)
			return
		}
		if renameResult {
			task.Tracef("mv %q %q", tmpDest, pkg.Dest)
			err = errors.WithStack(os.Rename(tmpDest, pkg.Dest))
		}
	}()

	f, r, mime, err := openArchive(source)
	if err != nil {
		return finalise, err
	}
	defer f.Close() // nolint: gosec

	info, err := f.Stat()
	if err != nil {
		return finalise, errors.WithStack(err)
	}

	task.Size(int(info.Size()))
	defer task.Done()
	r = io.NopCloser(io.TeeReader(r, task.ProgressWriter()))

	// Archive is a single executable.
	switch mime.String() {
	case "application/zip":
		return finalise, extractZip(task, f, info, tmpDest, pkg.Strip)

	case "application/x-7z-compressed":
		return finalise, extract7Zip(f, info.Size(), tmpDest, pkg.Strip)

	case "application/x-mach-binary", "application/x-elf",
		"application/x-executable", "application/x-sharedlib":
		return finalise, extractExecutable(r, tmpDest, path.Base(pkg.Source))

	case "application/x-tar":
		return finalise, extractPackageTarball(task, r, tmpDest, pkg.Strip)

	case "application/vnd.debian.binary-package":
		renameResult = false
		return finalise, extractDebianPackage(task, r, tmpDest, pkg)

	case "application/x-rpm":
		return finalise, extractRpmPackage(r, tmpDest, pkg)

	default:
		return finalise, errors.Errorf("don't know how to extract archive %s of type %s", source, mime)
	}

}

type hdiEntry struct {
	DevEntry   string `plist:"dev-entry"`
	MountPoint string `plist:"mount-point"`
}

type hdi struct {
	SystemEntities []*hdiEntry `plist:"system-entities"`
}

func isDirectory(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false, errors.WithStack(err)
	}
	return fileInfo.IsDir(), nil
}

func installFromDirectory(source string, pkg *manifest.Package) error {
	err := copy.Copy(source, pkg.Dest)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func installMacDMG(b *ui.Task, source string, pkg *manifest.Package) error {
	dest := pkg.Dest + "~"
	err := os.MkdirAll(dest, 0700)
	if err != nil {
		return errors.WithStack(err)
	}
	defer os.RemoveAll(dest)
	home, err := system.UserHomeDir()
	if err != nil {
		return errors.WithStack(err)
	}
	output, err := util.Capture(b, "hdiutil", "attach", "-plist", source)
	if err != nil {
		return errors.Wrap(err, "could not mount DMG")
	}
	list := &hdi{}
	_, err = plist.Unmarshal(output, list)
	if err != nil {
		return errors.WithStack(err)
	}
	var entry *hdiEntry
	for _, ent := range list.SystemEntities {
		if ent.MountPoint != "" && ent.DevEntry != "" {
			entry = ent
			break
		}
	}
	if entry == nil {
		return errors.New("couldn't determine volume information from hdiutil attach, volume may still be mounted :(")
	}
	defer util.Run(b, "hdiutil", "detach", entry.DevEntry) // nolint: errcheck
	switch {
	case len(pkg.Apps) != 0:
		for _, app := range pkg.Apps {
			base := filepath.Base(app)
			// Use rsync because reliably syncing all filesystem attributes is non-trivial.
			appDest := filepath.Join(dest, base)
			err = util.Run(b, "rsync", "-av",
				filepath.Join(entry.MountPoint, app)+"/",
				appDest+"/")
			if err != nil {
				return errors.WithStack(err)
			}
			err = os.Symlink(appDest, filepath.Join(home, "Applications", base))
			if err != nil {
				return errors.WithStack(err)
			}
		}
		return errors.WithStack(os.Rename(dest, pkg.Dest))

	default:
		return errors.New("manifest for does not provide a dmg{} block")
	}
}

func extractExecutable(r io.Reader, dest, executableName string) error {
	destExe := filepath.Join(dest, executableName)
	ext := filepath.Ext(destExe)
	switch ext {
	case ".gz", ".bz2", ".xz", ".zst":
		destExe = strings.TrimSuffix(destExe, ext)
	}

	w, err := os.OpenFile(destExe, os.O_CREATE|os.O_WRONLY, 0700) // nolint: gosec
	if err != nil {
		return errors.WithStack(err)
	}
	defer w.Close() // nolint: gosec
	_, err = io.Copy(w, r)
	return errors.WithStack(err)
}

// Open a potentially compressed archive.
//
// It will return the MIME type of the underlying file, and a buffered io.Reader for that file.
func openArchive(source string) (f *os.File, r io.Reader, mime *mimetype.MIME, err error) {
	mime, err = mimetype.DetectFile(source)
	if err != nil {
		return nil, nil, mime, errors.WithStack(err)
	}
	f, err = os.Open(source)
	if err != nil {
		return nil, nil, mime, errors.WithStack(err)
	}
	defer func() {
		if err != nil {
			_ = f.Close()
		}
	}()
	r = f
	switch mime.String() {
	case "application/gzip":
		zr, err := gzip.NewReader(r)
		if err != nil {
			return nil, nil, mime, errors.WithStack(err)
		}
		r = zr

	case "application/x-bzip2":
		r = bzip2.NewReader(r)

	case "application/x-xz":
		xr, err := xz.NewReader(r, 0)
		if err != nil {
			return nil, nil, mime, errors.WithStack(err)
		}
		r = xr

	case "application/zstd":
		zr, err := zstd.NewReader(r)
		if err != nil {
			return nil, nil, nil, errors.WithStack(err)
		}
		r = zr

	default:
		// Assume it's uncompressed?
		return f, r, mime, nil
	}

	// Now detect the underlying file type.
	buf := make([]byte, 4096)
	n, err := r.Read(buf)
	if err != nil && (!errors.Is(err, io.EOF) || n == 0) {
		return nil, nil, mime, errors.WithStack(err)
	}
	buf = buf[:n]
	mime = mimetype.Detect(buf)
	return f, io.MultiReader(bytes.NewReader(buf), r), mime, nil
}

const extractMacPkgChangesXML = `
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <array>
    <dict>
      <key>choiceAttribute</key>
      <string>customLocation</string>
      <key>attributeSetting</key>
      <string>${dest}</string>
      <key>choiceIdentifier</key>
      <string>default</string>
    </dict>
  </array>
</plist>
`

func extractMacPKG(b *ui.Task, path, dest string, strip int) error {
	if strip != 0 {
		return errors.Errorf("\"strip = %d\" is not supported for Mac installer .pkg files", strip)
	}
	err := os.MkdirAll(dest, 0700)
	if err != nil {
		return errors.WithStack(err)
	}
	task := b.SubProgress("install", 2)
	defer task.Done()
	changesf, err := ioutil.TempFile("", "hermit-*.xml")
	if err != nil {
		return errors.WithStack(err)
	}
	defer changesf.Close() // nolint: gosec
	defer os.Remove(changesf.Name())
	fmt.Fprint(changesf, os.Expand(extractMacPkgChangesXML, func(s string) string { return dest }))
	_ = changesf.Close()
	task.Add(1)
	return util.Run(b, "installer", "-verbose",
		"-pkg", path,
		"-target", "CurrentUserHomeDirectory",
		"-applyChoiceChangesXML", changesf.Name())
}

func extractZip(b *ui.Task, f *os.File, info os.FileInfo, dest string, strip int) error {
	zr, err := zip.NewReader(bufra.NewBufReaderAt(f, int(info.Size())), info.Size())
	if err != nil {
		return errors.WithStack(err)
	}
	task := b.SubProgress("unpack", len(zr.File))
	defer task.Done()
	for _, zf := range zr.File {
		b.Tracef("  %s", zf.Name)
		task.Add(1)
		destFile, err := makeDestPath(dest, zf.Name, strip)
		if err != nil {
			return err
		}
		if destFile == "" {
			continue
		}
		err = extractZipFile(zf, destFile)
		if err != nil {
			return errors.Wrap(err, destFile)
		}
	}
	return nil
}

func extractZipFile(zf *zip.File, destFile string) error {
	zfr, err := zf.Open()
	if err != nil {
		return errors.WithStack(err)
	}
	defer zfr.Close()
	if zf.Mode().IsDir() {
		return errors.WithStack(os.MkdirAll(destFile, 0700))
	}
	// Handle symlinks.
	if zf.Mode()&os.ModeSymlink != 0 {
		symlink, err := ioutil.ReadAll(zfr)
		if err != nil {
			return errors.WithStack(err)
		}
		dir := filepath.Dir(destFile)
		symlinkPath := filepath.Join(dir, string(symlink))
		symlinkPath, err = filepath.Rel(dir, symlinkPath)
		if err != nil {
			return errors.WithStack(err)
		}
		return errors.WithStack(os.Symlink(symlinkPath, destFile))
	}

	w, err := os.OpenFile(destFile, os.O_CREATE|os.O_WRONLY, zf.Mode()&^0077)
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = io.Copy(w, zfr) // nolint: gosec
	if err != nil {
		return errors.WithStack(err)
	}
	err = w.Close()
	if err != nil {
		return errors.WithStack(err)
	}
	_ = os.Chtimes(destFile, zf.Modified, zf.Modified) // Best effort.
	return nil
}

func extractPackageTarball(b *ui.Task, r io.Reader, dest string, strip int) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return errors.WithStack(err)
		}
		mode := hdr.FileInfo().Mode() &^ 0077
		destFile, err := makeDestPath(dest, hdr.Name, strip)
		if err != nil {
			return err
		}
		if destFile == "" {
			continue
		}
		b.Tracef("  %s -> %s", hdr.Name, destFile)
		err = os.MkdirAll(filepath.Dir(destFile), 0700)
		if err != nil {
			return errors.WithStack(err)
		}
		switch {
		case mode.IsDir():
			err = os.MkdirAll(destFile, 0700)
			if err != nil {
				return errors.Wrapf(err, "%s: failed to create directory", destFile)
			}

		case mode&os.ModeSymlink != 0:
			err = syscall.Symlink(hdr.Linkname, destFile)
			if err != nil {
				return errors.Wrapf(err, "%s: failed to create symlink to %s", destFile, hdr.Linkname)
			}

		case hdr.Typeflag&(tar.TypeLink|tar.TypeGNULongLink) != 0 && hdr.Linkname != "":
			// Convert hard links into symlinks so we don't have to track inodes later on during relocation.
			src := filepath.Join(dest, hdr.Linkname) // nolint: gosec
			rp, err := filepath.Rel(filepath.Dir(destFile), src)
			if err != nil {
				return errors.WithStack(err)
			}
			err = os.Symlink(rp, destFile)
			if err != nil {
				return errors.WithStack(err)
			}

		default:
			err := os.MkdirAll(filepath.Dir(destFile), 0700)
			if err != nil {
				return errors.WithStack(err)
			}
			w, err := os.OpenFile(destFile, os.O_CREATE|os.O_WRONLY, mode)
			if err != nil {
				return errors.WithStack(err)
			}
			_, err = io.Copy(w, tr) // nolint: gosec
			_ = w.Close()
			if err != nil {
				return errors.WithStack(err)
			}
			_ = os.Chtimes(destFile, hdr.AccessTime, hdr.ModTime) // Best effort.
		}
	}
	return nil
}

func extractDebianPackage(b *ui.Task, r io.Reader, dest string, pkg *manifest.Package) error {
	reader := ar.NewReader(r)
	for {
		header, err := reader.Next()
		if err != nil {
			return errors.WithStack(err)
		}
		if strings.HasPrefix(header.Name, "data.tar") {
			r := io.LimitReader(reader, header.Size)
			filename := filepath.Join(dest, header.Name)
			w, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
			if err != nil {
				return errors.WithStack(err)
			}
			_, err = io.Copy(w, r)
			_ = w.Close()
			if err != nil {
				return errors.WithStack(err)
			}
			_, err = Extract(b, filename, pkg)
			return err
		}
	}
}

func extract7Zip(r io.ReaderAt, size int64, dest string, strip int) error {
	sz, err := go7z.NewReader(r, size)
	if err != nil {
		return errors.WithStack(err)
	}

	for {
		hdr, err := sz.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		if err != nil {
			return errors.WithStack(err)
		}

		// If empty stream (no contents) and isn't specifically an empty file...
		// then it's a directory.
		if hdr.IsEmptyStream && !hdr.IsEmptyFile {
			continue
		}
		destFile, err := makeDestPath(dest, hdr.Name, strip)
		if err != nil {
			return err
		}
		if destFile == "" {
			continue
		}
		err = ensureDirExists(destFile)
		if err != nil {
			return errors.WithStack(err)
		}

		// Create file
		f, err := os.OpenFile(destFile, os.O_CREATE|os.O_RDWR, 0755) // nolint: gosec
		if err != nil {
			return errors.WithStack(err)
		}

		if _, err := io.Copy(f, sz); err != nil {
			_ = f.Close()
			return errors.WithStack(err)
		}
		if err = f.Close(); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func extractRpmPackage(r io.Reader, dest string, pkg *manifest.Package) error {
	rpm, err := rpmutils.ReadRpm(r)
	if err != nil {
		return errors.WithStack(err)
	}
	pr, err := rpm.PayloadReader()
	if err != nil {
		return errors.WithStack(err)
	}
	for {
		header, err := pr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errors.WithStack(err)
		}
		if header.Filesize() > 0 {
			bts := make([]byte, header.Filesize())
			_, err = pr.Read(bts)
			if err != nil {
				return errors.WithStack(err)
			}
			filename, err := makeDestPath(dest, header.Filename(), pkg.Strip)
			if err != nil {
				return err
			}
			if filename == "" {
				continue
			}
			err = ensureDirExists(filename)
			if err != nil {
				return errors.WithStack(err)
			}
			err = ioutil.WriteFile(filename, bts, os.FileMode(header.Mode()))
			if err != nil {
				return errors.WithStack(err)
			}
		}
	}
	return nil
}

func ensureDirExists(file string) error {
	dir := filepath.Dir(file)
	return os.MkdirAll(dir, os.ModePerm)
}

// Strip leading path component.
func makeDestPath(dest, path string, strip int) (string, error) {
	if err := sanitizeExtractPath(path, dest); err != nil {
		return "", err
	}
	parts := strings.Split(path, "/")
	if len(parts) <= strip {
		return "", nil
	}
	destFile := strings.Join(parts[strip:], "/")
	destFile = filepath.Join(dest, destFile)
	return destFile, nil
}

// https://snyk.io/research/zip-slip-vulnerability
func sanitizeExtractPath(filePath string, destination string) error {
	destPath := filepath.Join(destination, filePath)
	if !strings.HasPrefix(destPath, filepath.Clean(destination)) {
		return errors.Errorf("%s: illegal file path (%s not under %s)", filePath, destPath, destination)
	}
	return nil
}
