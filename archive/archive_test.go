package archive

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/ui"
)

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

			dest, err := ioutil.TempDir("", "")
			require.NoError(t, err)
			defer os.RemoveAll(dest)

			dest = filepath.Join(dest, "extracted")
			finalise, err := Extract(
				p.Task("extract"),
				filepath.Join("testdata", test.file),
				&manifest.Package{Dest: dest, Source: test.file},
			)
			require.NoError(t, err)
			require.NoError(t, finalise())
			for _, expected := range test.expected {
				info, err := os.Stat(filepath.Join(dest, expected))
				require.NoError(t, err)
				require.True(t, info.Mode()&unix.S_IXUSR != 0, "is not executable")
			}
		})
	}
}
