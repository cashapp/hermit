package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/ui"
)

func makeWritable(path string) error {
	return filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chmod(path, info.Mode()|0200) // Add write permission
	})
}

func TestExtract(t *testing.T) {
	tests := []struct {
		file     string
		expected []string
	}{
		{"archive.7z", []string{"darwin_exe", "linux_exe"}},
		{"archive.tar.bz2", []string{"darwin_exe", "linux_exe"}},
		{"archive.tar.gz", []string{"darwin_exe", "linux_exe"}},
		{"archive.tar.xz", []string{"darwin_exe", "linux_exe"}},
		{"archive.zip", []string{"darwin_exe", "linux_exe"}},
		{"darwin_exe", []string{"darwin_exe"}},
		{"linux_exe", []string{"linux_exe"}},
		{"darwin_exe.gz", []string{"darwin_exe"}},
		{"linux_exe.gz", []string{"linux_exe"}},
		{"bzip2_1.0.6-9.2_deb10u1_amd64.deb", []string{"/bin/bzip2"}},
		{"bzip2-1.0.6-13.el7.x86_64.rpm", []string{"/usr/bin/bzip2"}},
		{"directory", []string{"foo"}},
	}
	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			p, _ := ui.NewForTesting()

			tmpDir := t.TempDir()
			dest := filepath.Join(tmpDir, "extracted")
			t.Cleanup(func() {
				_ = makeWritable(tmpDir)
			})

			finalise, err := Extract(
				p.Task("extract"),
				filepath.Join("testdata", test.file),
				&manifest.Package{Dest: dest, Source: test.file},
			)
			assert.NoError(t, err)
			assert.NoError(t, finalise())
			for _, expected := range test.expected {
				info, err := os.Stat(filepath.Join(dest, expected))
				assert.NoError(t, err)
				assert.True(t, info.Mode()&unix.S_IXUSR != 0, "is not executable")
			}
		})
	}
}

// TestLinkTraversal tests that extracting archives with symlinks or hardlinks pointing
// outside the destination directory fails and doesn't create any files outside the dest.
func TestLinkTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a nested destination two levels deep so we can detect escapes
	// Structure: tmpDir/nested/extracted/
	nestedDir := filepath.Join(tmpDir, "nested")
	err := os.MkdirAll(nestedDir, 0750)
	assert.NoError(t, err)

	// Create the malicious tarball with both symlink and hardlink escapes
	maliciousTarball := filepath.Join(tmpDir, "malicious.tar.gz")
	f, err := os.Create(maliciousTarball)
	assert.NoError(t, err)

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// Add a regular file first
	err = tw.WriteHeader(&tar.Header{
		Name: "safe_file.txt",
		Mode: 0644,
		Size: 12,
	})
	assert.NoError(t, err)
	_, err = tw.Write([]byte("safe content"))
	assert.NoError(t, err)

	// Add a malicious symlink that points one directory up (escaping dest)
	err = tw.WriteHeader(&tar.Header{
		Name:     "evil_symlink",
		Mode:     0777,
		Typeflag: tar.TypeSymlink,
		Linkname: "../escape_marker",
	})
	assert.NoError(t, err)

	// Add a malicious hardlink that points one directory up (escaping dest)
	err = tw.WriteHeader(&tar.Header{
		Name:     "evil_hardlink",
		Mode:     0644,
		Typeflag: tar.TypeLink,
		Linkname: "../escape_marker",
	})
	assert.NoError(t, err)

	assert.NoError(t, tw.Close())
	assert.NoError(t, gw.Close())
	assert.NoError(t, f.Close())

	// Try to extract the malicious tarball into nested/extracted
	p, _ := ui.NewForTesting()
	dest := filepath.Join(nestedDir, "extracted")

	_, err = Extract(
		p.Task("extract"),
		maliciousTarball,
		&manifest.Package{Dest: dest, Source: "malicious.tar.gz"},
	)

	// Extraction should fail with an error about illegal link path
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "illegal") || strings.Contains(err.Error(), "symlink"),
		"expected error about illegal link path, got: %v", err)

	// Walk the entire tmpDir to verify nothing escaped
	// Only the tarball and nested directory should exist at tmpDir level
	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Get relative path from tmpDir
		relPath, err := filepath.Rel(tmpDir, path)
		if err != nil {
			return err
		}
		// Skip the root
		if relPath == "." {
			return nil
		}
		// Allow only: malicious.tar.gz, nested/, nested/extracted/
		// Nothing should exist in nested/ besides extracted/ (and its contents)
		allowedPrefixes := []string{"malicious.tar.gz", "nested"}
		allowed := false
		for _, prefix := range allowedPrefixes {
			if strings.HasPrefix(relPath, prefix) {
				allowed = true
				break
			}
		}
		assert.True(t, allowed, "unexpected file outside extraction directory: %s", path)
		// Specifically check that no "escape_marker" file was created
		assert.False(t, strings.Contains(relPath, "escape_marker"), "symlink/hardlink escape detected: %s", path)
		return nil
	})
	assert.NoError(t, err)
}

// TestLinkTraversalWithStrip tests that symlinks don't escape when strip is applied.
// Archive contains: foo/bar -> ../waz and foo/waz
// With strip=1, this becomes: bar -> ../waz which would escape if not handled properly.
func TestLinkTraversalWithStrip(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a nested destination two levels deep so we can detect escapes
	nestedDir := filepath.Join(tmpDir, "nested")
	err := os.MkdirAll(nestedDir, 0750)
	assert.NoError(t, err)

	// Create tarball with internal symlink that escapes after stripping
	tarball := filepath.Join(tmpDir, "strip_escape.tar.gz")
	f, err := os.Create(tarball)
	assert.NoError(t, err)

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// Add foo/ directory
	err = tw.WriteHeader(&tar.Header{
		Name:     "foo/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	})
	assert.NoError(t, err)

	// Add foo/waz file (the symlink target)
	err = tw.WriteHeader(&tar.Header{
		Name: "foo/waz",
		Mode: 0644,
		Size: 11,
	})
	assert.NoError(t, err)
	_, err = tw.Write([]byte("waz content"))
	assert.NoError(t, err)

	// Add foo/bar -> ../waz symlink
	// After strip=1, this becomes bar -> ../waz which escapes!
	err = tw.WriteHeader(&tar.Header{
		Name:     "foo/bar",
		Mode:     0777,
		Typeflag: tar.TypeSymlink,
		Linkname: "../waz",
	})
	assert.NoError(t, err)

	assert.NoError(t, tw.Close())
	assert.NoError(t, gw.Close())
	assert.NoError(t, f.Close())

	// Try to extract with strip=1 into nested/extracted
	p, _ := ui.NewForTesting()
	dest := filepath.Join(nestedDir, "extracted")

	_, err = Extract(
		p.Task("extract"),
		tarball,
		&manifest.Package{Dest: dest, Source: "strip_escape.tar.gz", Strip: 1},
	)

	// Extraction should fail because the symlink escapes after stripping
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "illegal") || strings.Contains(err.Error(), "symlink"),
		"expected error about illegal link path, got: %v", err)

	// Walk to verify nothing escaped
	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(tmpDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		// Should not find "waz" outside of nested/extracted
		if relPath == "waz" || relPath == "nested/waz" {
			t.Errorf("symlink escape detected: %s", path)
		}
		return nil
	})
	assert.NoError(t, err)
}

// TestZipInternalSymlink tests that relative symlinks pointing to sibling directories
// within the archive are allowed. This is the pattern used by packages like bats-core
// which contain test fixtures with symlinks like ../recursive/subsuite.
func TestZipInternalSymlink(t *testing.T) {
	tmpDir := t.TempDir()

	zipPath := filepath.Join(tmpDir, "internal_symlink.zip")
	f, err := os.Create(zipPath)
	assert.NoError(t, err)

	zw := zip.NewWriter(f)

	// Add directory: suite/recursive/
	_, err = zw.Create("suite/recursive/")
	assert.NoError(t, err)

	// Add file: suite/recursive/test.bats
	w, err := zw.Create("suite/recursive/test.bats")
	assert.NoError(t, err)
	_, err = w.Write([]byte("test content"))
	assert.NoError(t, err)

	// Add directory: suite/recursive/subsuite/
	_, err = zw.Create("suite/recursive/subsuite/")
	assert.NoError(t, err)

	// Add file: suite/recursive/subsuite/sub.bats
	w, err = zw.Create("suite/recursive/subsuite/sub.bats")
	assert.NoError(t, err)
	_, err = w.Write([]byte("sub content"))
	assert.NoError(t, err)

	// Add directory: suite/recursive_with_symlinks/
	_, err = zw.Create("suite/recursive_with_symlinks/")
	assert.NoError(t, err)

	// Add symlink: suite/recursive_with_symlinks/subsuite -> ../recursive/subsuite
	header := &zip.FileHeader{
		Name: "suite/recursive_with_symlinks/subsuite",
	}
	header.SetMode(os.ModeSymlink | 0777)
	w, err = zw.CreateHeader(header)
	assert.NoError(t, err)
	_, err = w.Write([]byte("../recursive/subsuite"))
	assert.NoError(t, err)

	assert.NoError(t, zw.Close())
	assert.NoError(t, f.Close())

	p, _ := ui.NewForTesting()
	dest := filepath.Join(tmpDir, "extracted")

	_, err = Extract(
		p.Task("extract"),
		zipPath,
		&manifest.Package{Dest: dest, Source: "internal_symlink.zip"},
	)
	assert.NoError(t, err, "internal symlinks within the archive should be allowed")

	// Verify the symlink was created and points to the right target
	target, err := os.Readlink(filepath.Join(dest, "suite", "recursive_with_symlinks", "subsuite"))
	assert.NoError(t, err)
	assert.Equal(t, "../recursive/subsuite", target)
}

// TestZipEscapingSymlink tests that symlinks in zip archives that escape
// the extraction root are rejected.
func TestZipEscapingSymlink(t *testing.T) {
	tmpDir := t.TempDir()

	zipPath := filepath.Join(tmpDir, "escaping_symlink.zip")
	f, err := os.Create(zipPath)
	assert.NoError(t, err)

	zw := zip.NewWriter(f)

	header := &zip.FileHeader{
		Name: "evil",
	}
	header.SetMode(os.ModeSymlink | 0777)
	w, err := zw.CreateHeader(header)
	assert.NoError(t, err)
	_, err = w.Write([]byte("../../etc/passwd"))
	assert.NoError(t, err)

	assert.NoError(t, zw.Close())
	assert.NoError(t, f.Close())

	p, _ := ui.NewForTesting()
	dest := filepath.Join(tmpDir, "extracted")

	_, err = Extract(
		p.Task("extract"),
		zipPath,
		&manifest.Package{Dest: dest, Source: "escaping_symlink.zip"},
	)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "illegal symlink target"),
		"expected error about illegal symlink target, got: %v", err)
}
