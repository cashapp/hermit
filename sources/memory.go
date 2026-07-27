package sources

import (
	"io/fs"

	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/vfs"
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

func (s *MemSource) Sync(_ *ui.UI, _ bool) (bool, error) {
	// See the equivalent comment on BuiltInSource.Sync: this source performs
	// no actual synchronisation, so it must report "false" here or it
	// poisons Sources.isSynchronised for every other source.
	return false, nil
}

func (s *MemSource) URI() string {
	return s.name
}

func (s *MemSource) Bundle() fs.FS {
	return vfs.InMemoryFS(map[string]string{
		s.name: s.content,
	})
}
