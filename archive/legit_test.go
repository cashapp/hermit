package archive

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/ui"
)

func writeTar(t *testing.T, path string, write func(tw *tar.Writer)) {
	f, err := os.Create(path)
	assert.NoError(t, err)
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	write(tw)
	assert.NoError(t, tw.Close())
	assert.NoError(t, gw.Close())
	assert.NoError(t, f.Close())
}

// 1. Directory symlink with files written *through* it (common "current -> versions/x" layout).
func TestLegitDirSymlinkWriteThrough(t *testing.T) {
	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "extracted")
	tarball := filepath.Join(tmpDir, "p.tar.gz")
	writeTar(t, tarball, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "versions/1.0/", Typeflag: tar.TypeDir, Mode: 0755})
		_ = tw.WriteHeader(&tar.Header{Name: "current", Typeflag: tar.TypeSymlink, Linkname: "versions/1.0", Mode: 0777})
		_ = tw.WriteHeader(&tar.Header{Name: "current/bin/tool", Typeflag: tar.TypeReg, Mode: 0755, Size: 2})
		_, _ = tw.Write([]byte("hi"))
	})

	p, _ := ui.NewForTesting()
	t.Cleanup(func() { _ = makeWritable(tmpDir) })
	_, err := Extract(p.Task("x"), tarball, &manifest.Package{Dest: dest, Source: "p.tar.gz"})
	assert.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(dest, "versions", "1.0", "bin", "tool"))
	assert.NoError(t, err)
	assert.Equal(t, "hi", string(b))
}

// 2. Legit relative symlink to a sibling file within the package.
func TestLegitRelativeSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "extracted")
	tarball := filepath.Join(tmpDir, "p.tar.gz")
	writeTar(t, tarball, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "lib/libfoo.so.1.2.3", Typeflag: tar.TypeReg, Mode: 0644, Size: 3})
		_, _ = tw.Write([]byte("lib"))
		_ = tw.WriteHeader(&tar.Header{Name: "lib/libfoo.so.1", Typeflag: tar.TypeSymlink, Linkname: "libfoo.so.1.2.3", Mode: 0777})
	})

	p, _ := ui.NewForTesting()
	t.Cleanup(func() { _ = makeWritable(tmpDir) })
	_, err := Extract(p.Task("x"), tarball, &manifest.Package{Dest: dest, Source: "p.tar.gz"})
	assert.NoError(t, err)
	target, err := os.Readlink(filepath.Join(dest, "lib", "libfoo.so.1"))
	assert.NoError(t, err)
	assert.Equal(t, "libfoo.so.1.2.3", target)
}

// 3. Legit dangling symlink whose target stays within the package (e.g. created at runtime).
func TestLegitDanglingInRootSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "extracted")
	tarball := filepath.Join(tmpDir, "p.tar.gz")
	writeTar(t, tarball, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "var/run", Typeflag: tar.TypeSymlink, Linkname: "state/run", Mode: 0777})
	})

	p, _ := ui.NewForTesting()
	t.Cleanup(func() { _ = makeWritable(tmpDir) })
	_, err := Extract(p.Task("x"), tarball, &manifest.Package{Dest: dest, Source: "p.tar.gz"})
	assert.NoError(t, err, "dangling symlink within the package must be allowed")
}

// 4. Circular symlink (a -> b, b -> a): broken but contained within the package, so
// it must not be rejected as an escape.
func TestCircularSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "extracted")
	tarball := filepath.Join(tmpDir, "p.tar.gz")
	writeTar(t, tarball, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "a", Typeflag: tar.TypeSymlink, Linkname: "b", Mode: 0777})
		_ = tw.WriteHeader(&tar.Header{Name: "b", Typeflag: tar.TypeSymlink, Linkname: "a", Mode: 0777})
	})

	p, _ := ui.NewForTesting()
	t.Cleanup(func() { _ = makeWritable(tmpDir) })
	_, err := Extract(p.Task("x"), tarball, &manifest.Package{Dest: dest, Source: "p.tar.gz"})
	assert.NoError(t, err, "contained circular symlinks must not be rejected")
}
