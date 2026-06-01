package sources

import (
	"io/fs"

	"github.com/cashapp/hermit/ui"
)

// LocalSource is a new Source based on a local filesystem
type LocalSource struct {
	fs *uriFS
}

// NewLocalSource returns a new LocalSource
func NewLocalSource(uri string, f fs.FS) *LocalSource {
	return &LocalSource{&uriFS{
		uri: uri,
		FS:  f,
	}}
}

func (s *LocalSource) Sync(_ *ui.UI, _ bool) (bool, error) {
	return true, nil
}

func (s *LocalSource) URI() string {
	return s.fs.uri
}

func (s *LocalSource) Bundle() fs.FS {
	return s.fs
}
