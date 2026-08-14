package sources

import (
	"io/fs"

	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/vfs"
)

// MemSource is a new Source based on a name and content kept in memory
type MemSource struct {
	name    SourceURI
	content string
}

// NewMemSource returns a new MemSource
func NewMemSource(name, content string) *MemSource {
	return &MemSource{NewSourceURI(name), content}
}

func (s *MemSource) Sync(_ *ui.UI, _ bool) (bool, error) {
	return true, nil
}

func (s *MemSource) URI() SourceURI {
	return s.name
}

func (s *MemSource) Bundle() fs.FS {
	return vfs.InMemoryFS(map[string]string{
		s.name.Get(): s.content,
	})
}
