package sources

import (
	"io/fs"

	"github.com/cashapp/hermit/ui"
)

// BuiltInSource is a source for built in packages
type BuiltInSource struct {
	fs fs.FS
}

// NewBuiltInSource returns a new MemSource
func NewBuiltInSource(dir fs.FS) *BuiltInSource {
	return &BuiltInSource{dir}
}

func (s *BuiltInSource) Sync(_ *ui.UI, _ bool) (bool, error) {
	return true, nil
}

func (s *BuiltInSource) URI() string {
	return "builtin:///"
}

func (s *BuiltInSource) Bundle() fs.FS {
	return &uriFS{s.URI(), s.fs}
}
