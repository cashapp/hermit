package state_test

import (
	"io/fs"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/vfs"
)

type StateTestFixture struct {
	Server *httptest.Server

	ui      *ui.UI
	root    string
	handler http.Handler
	roots   map[string]bool
	t       *testing.T
}

func NewStateTestFixture(t *testing.T) *StateTestFixture {
	t.Helper()
	ui, _ := ui.NewForTesting()
	return &StateTestFixture{
		t:     t,
		ui:    ui,
		roots: map[string]bool{},
	}
}

func (f *StateTestFixture) Clean() {
	f.t.Helper()
	if f.Server != nil {
		f.Server.Close()
	}
	for r := range f.roots {
		_ = filepath.Walk(r, func(path string, info fs.FileInfo, err error) error {
			_ = os.Chmod(path, 0700) // nolint
			return nil
		})
		err := os.RemoveAll(r)
		require.NoError(f.t, err)
	}
}

func (f *StateTestFixture) State() *state.State {
	root := f.root
	if root == "" {
		nroot, err := ioutil.TempDir("", "")
		require.NoError(f.t, err)
		root = nroot
	}

	if f.Server == nil {
		f.Server = httptest.NewServer(f.handler)
	}
	f.roots[root] = true
	sta, err := state.Open(root, state.Config{
		Builtin: sources.NewBuiltInSource(vfs.InMemoryFS(nil)),
	}, f.Server.Client(), f.Server.Client())
	require.NoError(f.t, err)
	return sta
}

func (f *StateTestFixture) WithHTTPHandler(handler http.Handler) *StateTestFixture {
	f.handler = handler
	return f
}
