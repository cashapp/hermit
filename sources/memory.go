package sources

import (
	"encoding/json"
	"fmt"
	"io/fs"

	"github.com/cashapp/hermit/sources/datauri"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/vfs"
)

// MemSource is a new Source based on a name and content kept in memory
type MemSource map[string]string

// NewMemSource returns a new MemSource
func NewMemSource(name, content string) MemSource {
	return NewMemSources(map[string]string{name: content})
}

// NewMemSources returns a new MemSource
func NewMemSources(sources map[string]string) MemSource {
	return sources
}

func (MemSource) Sync(_ *ui.UI, _ bool) error { // nolint: golint
	return nil
}

func (s MemSource) URI() string { // nolint: golint
	marshalled, err := json.Marshal(s)
	if err != nil {
		panic(fmt.Sprintf("json.Marshalling a map[string]string should never fail: %s", err))
	}
	return datauri.Encode(marshalled)
}

func (s MemSource) Bundle() fs.FS { // nolint: golint
	return vfs.InMemoryFS(s)
}
