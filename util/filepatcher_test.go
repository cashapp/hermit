package util

import (
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestFilePatcher_patches_existing_files_more(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		patch    string
		expected string
		changed  bool
	}{{
		name:    "patches with longer content",
		patch:   "foobar\nfoo\nbar",
		changed: true,
		contents: `
foo
#START
bar
#END
foobar
`,
		expected: `
foo
#START
foobar
foo
bar
#END
foobar
`,
	}, {
		name:    "patches with shorter content",
		patch:   "foobar",
		changed: true,
		contents: `
#START
bar
foo
#END
foobar
`,
		expected: `
#START
foobar
#END
foobar
`,
	}, {
		name:    "returns no change",
		patch:   "foobar",
		changed: false,
		contents: `
foo
#START
foobar
#END
`,
		expected: `
foo
#START
foobar
#END
`,
	}, {
		name:    "creates the file if it does not exist",
		patch:   "foobar",
		changed: true,
		expected: `
#START
foobar
#END
`,
	}}

	dir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			patcher := NewFilePatcher("#START", "#END")
			file := filepath.Join(dir, strconv.Itoa(rand.Int())) // nolint: gosec
			if test.contents != "" {
				file = fileWith(t, dir, test.contents)
			}
			changed, err := patcher.Patch(file, test.patch)
			require.Equal(t, test.changed, changed)
			require.NoError(t, err)

			bts, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Equal(t, test.expected, string(bts))
		})
	}
}

func fileWith(t *testing.T, dir, content string) (fileName string) {
	t.Helper()
	file, err := os.CreateTemp(dir, ".file")
	require.NoError(t, err)
	name := file.Name()
	err = ioutil.WriteFile(name, []byte(content), 0644) // nolint: gosec
	require.NoError(t, err)
	return name
}
