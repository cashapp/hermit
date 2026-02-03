package hermit_test

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/square/exit"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/hermittest"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/manifest/manifesttest"
)

// Test that when installing a package that has binaries conflicting
// with an existing package, we get an error
func TestConflictingBinariesError(t *testing.T) {
	fixture := hermittest.NewEnvTestFixture(t, nil)

	pkg1 := manifesttest.NewPkgBuilder(fixture.RootDir()).
		WithSource("archive/testdata/archive.tar.gz").
		Result()

	pkg2 := manifesttest.NewPkgBuilder(fixture.RootDir()).
		WithSource("archive/testdata/archive.tar.gz").
		WithName("test2").
		WithVersion("1").
		Result()

	_, err := fixture.Env.Install(fixture.P, pkg1)
	assert.NoError(t, err)

	_, err = fixture.Env.Install(fixture.P, pkg2)
	assert.EqualError(t, err, "test2-1 can not be installed, the following binaries already exist: darwin_exe, linux_exe")
}

// Test that the update timestamp and etag are written to the DB correctly when
// installing a package with an update interval
func TestUpdateTimestampOnInstall(t *testing.T) {
	calls := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("ETag", "testtag")
		dat, _ := os.ReadFile("archive/testdata/archive.tar.gz")
		_, err := w.Write(dat)
		assert.NoError(t, err)
		calls++
	})
	fixture := hermittest.NewEnvTestFixture(t, handler)

	pkg := manifesttest.NewPkgBuilder(fixture.RootDir()).
		WithName("test").
		WithChannel("stable").
		WithUpdateInterval(1 * time.Hour).
		WithSource(fixture.Server.URL).
		Result()

	_, err := fixture.Env.Install(fixture.P, pkg)
	assert.NoError(t, err)

	dbPkg := fixture.GetDBPackage("test@stable")
	actual := dbPkg.UpdateCheckedAt.Unix()
	assert.True(t, time.Now().Add(1*time.Minute).Unix() >= actual)
	assert.True(t, time.Now().Add(-1*time.Minute).Unix() <= actual)
	assert.Equal(t, "testtag", dbPkg.Etag)
	assert.Equal(t, 1, calls)
}

// Tests that EnsureUpToDate only updates the package when the etag has changed
func TestEnsureUpToDate(t *testing.T) {
	etag := "first"
	data := "data"
	headCalls := 0
	getCalls := 0
	fail := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("ETag", etag)
		if fail {
			w.WriteHeader(500)
		} else if r.Method == "HEAD" {
			headCalls++
		} else if r.Method == "GET" {
			getCalls++
			tar := TestTarGz{map[string]string{"bin": data}}
			tar.Write(t, w)
		}
	})
	fixture := hermittest.NewEnvTestFixture(t, handler)
	dao := fixture.DAO()

	pkg := manifesttest.NewPkgBuilder(fixture.RootDir()).
		WithName("test").
		WithBinaries("bin").
		WithChannel("chan").
		WithUpdateInterval(1 * time.Hour).
		WithSource(fixture.Server.URL).
		Result()

	// Initial install
	_, err := fixture.Env.Install(fixture.P, pkg)
	assert.NoError(t, err)
	assert.Equal(t, 1, getCalls)
	assert.Equal(t, 0, headCalls)

	// Update before update check is due
	err = fixture.Env.EnsureChannelIsUpToDate(fixture.P, pkg)
	assert.NoError(t, err)
	assert.Equal(t, 1, getCalls)
	assert.Equal(t, 0, headCalls)
	file, _ := os.ReadFile(filepath.Join(pkg.Dest, "bin"))
	assert.Equal(t, data, string(file))

	// Update after a check is needed but etag has not changed
	pkg.UpdatedAt = time.Now().Add(-2 * time.Hour)
	err = fixture.Env.EnsureChannelIsUpToDate(fixture.P, pkg)
	assert.NoError(t, err)
	assert.Equal(t, 1, getCalls)
	assert.Equal(t, 1, headCalls)
	file, _ = os.ReadFile(filepath.Join(pkg.Dest, "bin"))
	assert.Equal(t, data, string(file))

	// Update after a check is needed and the etag has changed
	pkg.UpdatedAt = time.Now().Add(-2 * time.Hour)
	etag = "changed"
	data = strings.Repeat("other", 1024)
	err = fixture.Env.EnsureChannelIsUpToDate(fixture.P, pkg)
	assert.NoError(t, err)
	assert.Equal(t, 2, getCalls)
	assert.Equal(t, 2, headCalls)
	file, _ = os.ReadFile(filepath.Join(pkg.Dest, "bin"))
	assert.Equal(t, data, string(file))

	// Check that the package is still in the DB after the upgrade
	dbPkg, err := dao.GetPackage(pkg.Reference.String())
	assert.NoError(t, err)
	assert.NotZero(t, dbPkg)

	// Check etag retained when the connection fails
	fail = true
	pkg.UpdatedAt = time.Now().Add(-2 * time.Hour)
	err = fixture.Env.EnsureChannelIsUpToDate(fixture.P, pkg)
	assert.NoError(t, err)
	dbPkg, err = dao.GetPackage(pkg.Reference.String())
	assert.NoError(t, err)
	assert.Equal(t, etag, dbPkg.Etag)
}

// Test that files referred in the Files map are copied correctly
func TestCopyFiles(t *testing.T) {
	dir := t.TempDir()

	f, err := os.Create(filepath.Join(dir, "from"))
	assert.NoError(t, err)
	err = f.Close()
	assert.NoError(t, err)

	fixture := hermittest.NewEnvTestFixture(t, nil)

	pkg := manifesttest.NewPkgBuilder(fixture.RootDir()).
		WithSource("archive/testdata/archive.tar.gz").
		WithVersion("1").
		WithFile("from", filepath.Join(dir, "to"), os.DirFS(dir)).
		Result()
	_, err = fixture.Env.Install(fixture.P, pkg)
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "to"))
	assert.NoError(t, err)
}

// Test that files referred in the Files map are copied correctly
func TestCopyFilesAction(t *testing.T) {
	dir := t.TempDir()

	f, err := os.Create(filepath.Join(dir, "from"))
	assert.NoError(t, err)
	err = f.Close()
	assert.NoError(t, err)

	fixture := hermittest.NewEnvTestFixture(t, nil)

	pkg := manifesttest.NewPkgBuilder(fixture.RootDir()).
		WithSource("archive/testdata/archive.tar.gz").
		WithVersion("1").
		WithFS(os.DirFS(dir)).
		WithTrigger(manifest.EventUnpack, &manifest.CopyAction{
			From: "from",
			To:   filepath.Join(dir, "to"),
			Mode: 0755,
		}).
		Result()
	_, err = fixture.Env.Install(fixture.P, pkg)
	assert.NoError(t, err)

	info, err := os.Stat(filepath.Join(dir, "to"))
	assert.NoError(t, err)
	assert.Equal(t, 0755, int(info.Mode()))
}

func TestTriggers(t *testing.T) {
	dir := t.TempDir()

	file := filepath.Join(dir, "test.sh")
	target := filepath.Join(dir, "success")

	fd, err := os.Create(file)
	assert.NoError(t, err)
	_, err = fd.WriteString("#!/bin/sh\ntouch " + target)
	assert.NoError(t, err)
	err = fd.Close()
	assert.NoError(t, err)

	fixture := hermittest.NewEnvTestFixture(t, nil)

	pkg := manifesttest.NewPkgBuilder(fixture.RootDir()).
		WithSource("archive/testdata/archive.tar.gz").
		WithVersion("1").
		WithTrigger(manifest.EventUnpack,
			&manifest.RunAction{
				Command: "/bin/sh",
				Dir:     dir,
				Args:    []string{file},
				Env:     nil,
				Stdin:   "",
			}).
		Result()
	_, err = fixture.Env.Install(fixture.P, pkg)
	assert.NoError(t, err)

	_, err = os.Stat(target)
	assert.NoError(t, err)
}

func TestExpandEnvars(t *testing.T) {
	tests := []struct {
		in       []string
		ops      []string
		expected []string
	}{
		{in: []string{
			"PATH=/usr/local/bin:/usr/bin",
			"HERMIT_STATE_DIR=/tmp/cache/hermit",
			"HERMIT_ENV=/tmp/env",
		},
			ops: []string{
				"NODE_PATH=${HERMIT_STATE_DIR}/pkg/node",
				"PATH=${HERMIT_ENV}/bin:${PATH}",
				"PATH=${NODE_PATH}/bin:${PATH}",
			},
			expected: []string{
				"HERMIT_ENV=/tmp/env",
				"HERMIT_STATE_DIR=/tmp/cache/hermit",
				"NODE_PATH=/tmp/cache/hermit/pkg/node",
				"PATH=/tmp/cache/hermit/pkg/node/bin:/tmp/env/bin:/usr/local/bin:/usr/bin",
			},
		},
		{in: []string{},
			ops: []string{
				"A=${B}",
				"B=${A}",
			},
			expected: []string{},
		},
	}
	for _, test := range tests {
		ops := envars.Infer(test.ops)
		actual := envars.Parse(test.in).
			Apply("", ops).
			Combined().
			System()
		assert.Equal(t, test.expected, actual)
	}
}

func TestDependencyResolution(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tar := TestTarGz{map[string]string{"bin1": "foo"}}
		tar.Write(t, w)
	})

	f := hermittest.NewEnvTestFixture(t, handler)
	f.WithManifests(map[string]string{
		"dep.hcl": `
			description = ""
			binaries = ["bin1"]
			version "1.0.0" {
			  source = "` + f.Server.URL + `"
			}
			provides = ["virtual"]
		`,
		"pkg1.hcl": `
			description = ""
			binaries = ["bin1"]
			version "1.0.0" {
			  source = "` + f.Server.URL + `"
			}
			requires = ["dep"]
			provides = ["virtual2"]
		`,
		"pkg2.hcl": `
			description = ""
			binaries = ["bin1"]
			version "1.0.0" {
			  source = "` + f.Server.URL + `"
			}
			requires = ["virtual"]
			provides = ["virtual2"]
		`,
		"pkg3.hcl": `
			description = ""
			binaries = ["bin1"]
			version "1.0.0" {
			  source = "` + f.Server.URL + `"
			}
			requires = ["not-found"]
		`,
		"pkg4.hcl": `
			description = ""
			binaries = ["bin1"]
			version "1.0.0" {
			  source = "` + f.Server.URL + `"
			}
			requires = ["virtual2"]
		`,
	})
	defer f.Clean()

	pkg, err := f.Env.Resolve(f.P, manifest.NameSelector("dep"), false)
	assert.NoError(t, err)
	_, err = f.Env.Install(f.P, pkg)
	assert.NoError(t, err)

	installed, err := f.Env.ListInstalledReferences()
	assert.NoError(t, err)

	// Test that dependencies can be resolved based on the package name
	err = f.Env.ResolveWithDeps(f.P, installed, manifest.NameSelector("pkg1"), map[string]*manifest.Package{})
	assert.NoError(t, err)

	// Test that dependencies can be resolved based on the virtual package name
	err = f.Env.ResolveWithDeps(f.P, installed, manifest.NameSelector("pkg2"), map[string]*manifest.Package{})
	assert.NoError(t, err)

	// Test that missing dependencies fail
	err = f.Env.ResolveWithDeps(f.P, installed, manifest.NameSelector("pkg3"), map[string]*manifest.Package{})
	assert.True(t, errors.Is(err, manifest.ErrUnknownPackage))

	// Test that resolving package where requirement is fulfilled by multiple uninstalled packages fails
	err = f.Env.ResolveWithDeps(f.P, installed, manifest.NameSelector("pkg4"), map[string]*manifest.Package{})
	assert.EqualError(t, err, "multiple packages satisfy the required dependency \"virtual2\", please install one of the following manually: pkg1, pkg2")

	// Test that an explicitly selected provider resolves virtual dependencies.
	selected := map[string]*manifest.Package{}
	err = f.Env.ResolveWithDeps(f.P, installed, manifest.NameSelector("pkg1"), selected)
	assert.NoError(t, err)
	err = f.Env.ResolveWithDeps(f.P, installed, manifest.NameSelector("pkg4"), selected)
	assert.NoError(t, err)

	// Test that multiple selected providers do not fail resolution.
	selected = map[string]*manifest.Package{}
	err = f.Env.ResolveWithDeps(f.P, installed, manifest.NameSelector("pkg1"), selected)
	assert.NoError(t, err)
	err = f.Env.ResolveWithDeps(f.P, installed, manifest.NameSelector("pkg2"), selected)
	assert.NoError(t, err)
	err = f.Env.ResolveWithDeps(f.P, installed, manifest.NameSelector("pkg4"), selected)
	assert.NoError(t, err)
}

func TestVirtualRuntimeDependencies(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tar := TestTarGz{map[string]string{"java": "java-binary", "consumer-bin": "consumer-binary"}}
		tar.Write(t, w)
	})

	f := hermittest.NewEnvTestFixture(t, handler)
	f.WithManifests(map[string]string{
		// Provider package that provides the "jdk" virtual capability
		"openjdk.hcl": `
			description = ""
			binaries = ["java"]
			version "21" {
			  source = "` + f.Server.URL + `"
			}
			provides = ["jdk"]
		`,
		// Consumer package that has runtime-dependencies on the virtual "jdk"
		"consumer.hcl": `
			description = ""
			binaries = ["consumer-bin"]
			version "1.0.0" {
			  source = "` + f.Server.URL + `"
			}
			runtime-dependencies = ["jdk"]
		`,
	})
	defer f.Clean()

	// Install the provider first
	provider, err := f.Env.Resolve(f.P, manifest.NameSelector("openjdk"), false)
	assert.NoError(t, err)
	_, err = f.Env.Install(f.P, provider)
	assert.NoError(t, err)

	// Now install the consumer - should succeed because openjdk provides "jdk"
	consumer, err := f.Env.Resolve(f.P, manifest.NameSelector("consumer"), false)
	assert.NoError(t, err)
	_, err = f.Env.Install(f.P, consumer)
	assert.NoError(t, err)
}

func TestVirtualRuntimeDependenciesNotInstalled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tar := TestTarGz{map[string]string{"java": "java-binary", "consumer-bin": "consumer-binary"}}
		tar.Write(t, w)
	})

	f := hermittest.NewEnvTestFixture(t, handler)
	f.WithManifests(map[string]string{
		// Provider package that provides the "jdk" virtual capability
		"openjdk.hcl": `
			description = ""
			binaries = ["java"]
			version "21" {
			  source = "` + f.Server.URL + `"
			}
			provides = ["jdk"]
		`,
		// Consumer package that has runtime-dependencies on the virtual "jdk"
		"consumer.hcl": `
			description = ""
			binaries = ["consumer-bin"]
			version "1.0.0" {
			  source = "` + f.Server.URL + `"
			}
			runtime-dependencies = ["jdk"]
		`,
	})
	defer f.Clean()

	// Try to install the consumer without installing the provider first
	// Should fail because no installed package provides "jdk"
	consumer, err := f.Env.Resolve(f.P, manifest.NameSelector("consumer"), false)
	assert.NoError(t, err)
	_, err = f.Env.Install(f.P, consumer)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "jdk")
}

func TestManifestValidation(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bar" {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	})

	f := hermittest.NewEnvTestFixture(t, handler)
	f.WithManifests(map[string]string{
		"test.hcl": `
			description = ""
			binaries = ["bin1"]
			version "1.0.0" {
		      linux { source = "` + f.Server.URL + `/foo" }
              darwin { source = "` + f.Server.URL + `/bar" }
			}
		`,
	})
	defer f.Clean()

	_, err := f.Env.ValidateManifest(f.P, "test", &hermit.ValidationOptions{CheckSources: true})
	assert.Error(t, err)
	assert.Equal(t, "test-1.0.0: darwin-amd64: invalid source: could not retrieve source archive from "+f.Server.URL+"/bar: 404 Not Found", err.Error())

	_, err = f.Env.ValidateManifest(f.P, "test", &hermit.ValidationOptions{CheckSources: false})
	assert.NoError(t, err)
}

func TestEnv_EphemeralVariableSubstitutionOverride(t *testing.T) {
	fixture := hermittest.NewEnvTestFixture(t, nil)
	defer fixture.Clean()

	err := fixture.Env.SetEnv("TOOL_HOME", "$HERMIT_ENV/.hermit/tool")
	assert.NoError(t, err)

	var envop envars.Op = &envars.Set{Name: "TOOL_HOME", Value: "$HERMIT_ENV/.hermit/tool"}
	ops, err := fixture.Env.EnvOps(fixture.P)
	assert.NoError(t, err)
	opsContains(t, ops, envop)

	vars, err := fixture.Env.Envars(fixture.P, false)
	assert.NoError(t, err)
	opsContains(t, vars, "TOOL_HOME="+fixture.Env.Root()+"/.hermit/tool")
}

func TestLoadEnvInfo(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		expected hermit.Config
	}{
		{
			name: "empty",
			expected: hermit.Config{
				Envars:    map[string]string{},
				ManageGit: true,
			},
		},
		{
			name: "github-token-auth",
			in: joinLines(
				`github-token-auth {`,
				`  match = ["cashapp/*"]`,
				`}`,
			),
			expected: hermit.Config{
				Envars:    map[string]string{},
				ManageGit: true,
				GitHubTokenAuth: hermit.GitHubTokenAuthConfig{
					Match: []string{
						"cashapp/*",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			// On macOS, the temporary directory path may be a symlink.
			// Resolve it so we can match it exactly later.
			root, err := filepath.EvalSymlinks(root)
			assert.NoError(t, err)

			binDir := filepath.Join(root, "bin")
			fpath := filepath.Join(binDir, "hermit.hcl")
			assert.NoError(t,
				os.MkdirAll(filepath.Dir(fpath), 0750))
			assert.NoError(t,
				os.WriteFile(fpath, []byte(tt.in), 0600))

			info, err := hermit.LoadEnvInfo(root)
			assert.NoError(t, err)

			assert.Equal(t, &tt.expected, info.Config)
			assert.Equal(t, root, info.Root)
			assert.Equal(t, binDir, info.BinDir)
		})
	}
}

func TestVerify_HermitEnv(t *testing.T) {
	fixture := hermittest.NewEnvTestFixture(t, nil)
	defer fixture.Clean()

	info, err := hermit.LoadEnvInfo(fixture.Env.EnvDir())
	assert.NoError(t, err)

	// Get script checksums from fixture
	scriptSums := fixture.ScriptSums()

	env, err := hermit.OpenEnv(info, fixture.State, nil, nil, nil, scriptSums)
	assert.NoError(t, err)

	err = env.Verify()
	assert.NoError(t, err)
}

func TestVerify_NonHermitEnv(t *testing.T) {
	fixture := hermittest.NewEnvTestFixture(t, nil)
	defer fixture.Clean()

	// Remove hermit binary which is required
	err := os.Remove(filepath.Join(fixture.Env.BinDir(), "hermit"))
	assert.NoError(t, err)

	err = fixture.Env.Verify()
	assert.Error(t, err)
	assert.Equal(t, exit.RequirementNotMet, exit.FromError(err))
}

func opsContains[T any](t *testing.T, slice []T, needle T) {
	t.Helper()
	for _, el := range slice {
		if reflect.DeepEqual(el, needle) {
			return
		}
	}
	t.Fatalf("%v does not contain %v", slice, needle)
}

// Helper to write multi-line strings inside a table test.
func joinLines(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}
