package archive

import (
	"archive/tar"
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
