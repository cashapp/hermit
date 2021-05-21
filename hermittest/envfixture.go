package hermittest

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/vfs"

	bolt "go.etcd.io/bbolt"

	"github.com/cashapp/hermit"

	"github.com/cashapp/hermit/state"

	"github.com/cashapp/hermit/internal/dao"
	"github.com/cashapp/hermit/ui"
	"github.com/stretchr/testify/require"
)

// EnvTestFixture encapsulates the directories used by Env and the Env itself
type EnvTestFixture struct {
	State   *state.State
	EnvDirs []string
	Env     *hermit.Env
	Logs    *bytes.Buffer
	Server  *httptest.Server
	P       *ui.UI
	t       *testing.T
}

// NewEnvTestFixture returns a new empty fixture with Env initialised to default values.
// A test handler can be given to be used as an test http server for testing http interactions
func NewEnvTestFixture(t *testing.T, handler http.Handler) *EnvTestFixture {
	t.Helper()
	envDir, err := ioutil.TempDir("", "")
	require.NoError(t, err)

	stateDir, err := ioutil.TempDir("", "")
	require.NoError(t, err)

	log, buf := ui.NewForTesting()

	err = hermit.Init(log, envDir, "", stateDir, hermit.Config{})
	require.NoError(t, err)

	server := httptest.NewServer(handler)
	sta, err := state.Open(stateDir, state.Config{
		Sources: []string{},
		Builtin: sources.NewBuiltInSource(vfs.InMemoryFS(nil)),
	}, server.Client(), server.Client(), log)
	require.NoError(t, err)
	env, err := hermit.OpenEnv(log, envDir, sta, envars.Envars{})
	require.NoError(t, err)

	return &EnvTestFixture{
		State:   sta,
		EnvDirs: []string{envDir},
		Logs:    buf,
		Env:     env,
		Server:  server,
		t:       t,
		P:       log,
	}
}

// RootDir returns the directory to the environment package root
func (f *EnvTestFixture) RootDir() string {
	return filepath.Join(f.State.Root(), "pkg")
}

// DAO returns the DAO using the underlying hermit database
func (f *EnvTestFixture) DAO() *dao.DAO {
	return dao.Open(f.State.Root())
}

// BoltDB returns the underlying DB
func (f *EnvTestFixture) BoltDB() *bolt.DB {
	db, err := bolt.Open(filepath.Join(f.State.Root(), "hermit.bolt.db"), 0600, nil)
	require.NoError(f.t, err)
	return db
}

// Clean removes all files and directories from this environment
func (f *EnvTestFixture) Clean() {
	for _, dir := range f.EnvDirs {
		os.RemoveAll(dir)
	}
	os.RemoveAll(f.State.Root())
	f.Server.Close()
}

// NewEnv returns a new environment using the state directory from this fixture
func (f *EnvTestFixture) NewEnv() *hermit.Env {
	p, _ := ui.NewForTesting()

	envDir, err := ioutil.TempDir("", "")
	require.NoError(f.t, err)
	log, _ := ui.NewForTesting()
	err = hermit.Init(log, envDir, "", f.State.Root(), hermit.Config{})
	require.NoError(f.t, err)
	env, err := hermit.OpenEnv(p, envDir, f.State, envars.Envars{})
	require.NoError(f.t, err)
	return env
}

// GetDBPackage return the data from the DB for a package
func (f *EnvTestFixture) GetDBPackage(ref string) *dao.Package {
	dao := f.DAO()
	dbPkg, err := dao.GetPackage(ref)
	require.NoError(f.t, err)
	return dbPkg
}

// WithManifests sets the resolver manifests for the current environment.
// Warning: any additional environments created from this fixture previously
// will not be updated.
func (f *EnvTestFixture) WithManifests(files map[string]string) *EnvTestFixture {
	for name, content := range files {
		err := f.Env.AddSource(f.P, sources.NewMemSource(name, content))
		require.NoError(f.t, err)
	}
	return f
}
