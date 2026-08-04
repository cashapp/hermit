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
	// This source performs no actual synchronisation, so "false" is the
	// correct answer to "did I actually update anything?" -- returning
	// "true" here poisons Sources.isSynchronised (sources.go), which is set
	// if *any* source reports it synced, and since BuiltInSource is always
	// prepended, that made every other source's "sync and retry" a no-op.
	return false, nil
}

func (s *BuiltInSource) URI() string {
	return "builtin:///"
}

func (s *BuiltInSource) Bundle() fs.FS {
	// dir is deliberately left empty: this source is backed by an in-memory
	// FS with no directory on disk that could vanish out from under it (see
	// the comment on uriFS.dir).
	return &uriFS{uri: s.URI(), FS: s.fs}
}
