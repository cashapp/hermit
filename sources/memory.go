package sources

import (
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/vfs"
	"io/fs"
)

// MemSource is a new Source based on a name and content kept in memory
type MemSource struct {
	name    string
	content string
}

// NewMemSource returns a new MemSource
func NewMemSource(name, content string) *MemSource {
	return &MemSource{name, content}
}

func (s *MemSource) Sync(_ *ui.UI, _ bool) error { // nolint: golint
	return nil
}

func (s *MemSource) URI() string { // nolint: golint
	return s.name
}

func (s *MemSource) Bundle() fs.FS { // nolint: golint
	return vfs.InMemoryFS(map[string]string{
		s.name: s.content,
	})
}
