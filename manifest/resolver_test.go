package manifest_test

import (
	"os"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/alecthomas/hcl"
	"github.com/alecthomas/repr"

	"github.com/cashapp/hermit/envars"
	. "github.com/cashapp/hermit/manifest" //nolint:revive // dot import
	"github.com/cashapp/hermit/manifest/manifesttest"
	"github.com/cashapp/hermit/platform"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/vfs"
)

func TestResolver_Resolve(t *testing.T) {
	config := Config{
		Env:   "/home/user/project",
		State: "/home/user/.cache/hermit",
		Platform: platform.Platform{
			OS:   platform.Linux,
			Arch: platform.Amd64,
		},
	}
	tests := []struct {
		name           string
		files          map[string]string
		manifestErrors map[string][]string
		reference      string
		wantPkg        *Package
		wantErr        string
	}{{
		name: "Update interval is parsed correctly",
		files: map[string]string{
			"testchan.hcl": `
                description = ""
				binaries = ["bin"]
				channel "stable" {
				  update = "5h"
				  source = "www.example.com"
				}
            `,
		},
		reference: "testchan@stable",
		wantPkg: manifesttest.NewPkgBuilder(config.State + "/pkg/testchan@stable").
			WithName("testchan").
			WithBinaries("bin").
			WithChannel("stable").
			WithSource("www.example.com").
			WithUpdateInterval(5 * time.Hour).
			Result(),
	}, {
		name: "Resolves to the latest version by default",
		files: map[string]string{
			"testchan.hcl": `
                description = ""
				binaries = ["bin"]
				version "0.1.0" {
				  source = "www.example-1.com"
				}
				version "1.0.0" {
				  source = "www.example-2.com"
				}
				version "0.0.1" {
				  source = "www.example-3.com"
				}
            `,
		},
		reference: "testchan",
		wantPkg: manifesttest.NewPkgBuilder(config.State + "/pkg/testchan-1.0.0").
			WithName("testchan").
			WithBinaries("bin").
			WithVersion("1.0.0").
			WithSource("www.example-2.com").
			Result(),
	}, {
		name: "Resolves triggers using correct version",
		files: map[string]string{
			"test.hcl": `
                description = ""
				binaries = ["bin"]
				on "unpack" {
					copy { from = "foo/bar" to = "${root}/fizz" }
					run { cmd = "/test" dir = "${root}" }
					copy { from = "foo/baz" to = "${root}/biz" }
					message { text = "hello" }
				}
				version "0.1.0" {
				  source = "www.example-1.com"
				}
				version "1.0.0" {
				  source = "www.example-2.com"
				}
            `,
		},
		reference: "test",
		wantPkg: manifesttest.NewPkgBuilder(config.State+"/pkg/test-1.0.0").
			WithName("test").
			WithBinaries("bin").
			WithVersion("1.0.0").
			WithSource("www.example-2.com").
			WithTrigger(EventUnpack,
				&CopyAction{From: "foo/bar", To: config.State + "/pkg/test-1.0.0/fizz"},
				&RunAction{
					Command: "/test",
					Dir:     config.State + "/pkg/test-1.0.0",
				},
				&CopyAction{From: "foo/baz", To: config.State + "/pkg/test-1.0.0/biz"},
				&MessageAction{Text: "hello"},
			).
			Result(),
	}, {
		name: "Suggest version for missing channel",
		files: map[string]string{
			"testchan.hcl": `
                description = ""
				binaries = ["bin"]
				version "1.0.0" {
				  source = "www.example.com"
				}
			`,
		},
		reference: "testchan@1.0.0",
		wantErr:   "memory:///testchan.hcl: no channel testchan@1.0.0 found, did you mean version testchan-1.0.0?: unknown package",
	}, {
		name: "Suggest version syntax for missing channel",
		files: map[string]string{
			"testchan.hcl": `
                description = ""
				binaries = ["bin"]
				version "1.0.0" {
				  source = "www.example.com"
				}
			`,
		},
		reference: "testchan@1.0.1",
		wantErr:   "memory:///testchan.hcl: no channel testchan@1.0.1 found in channels (testchan@1, testchan@1.0, testchan@latest) or versions (testchan-1.0.0): unknown package",
	}, {
		name: "Infer",
		files: map[string]string{
			"test.hcl": `
				description = ""
				binaries = ["bin"]
				env = {
					PATH: "${env}/bin:${PATH}",
					LD_LIBRARY_PATH: "${LD_LIBRARY_PATH}:${env}/lib",
					GOPATH: "${env}/go"
				}
				version "1.0.0" {
				  source = "www.example.com"
				}
			`,
		},
		reference: "test",
		wantPkg: manifesttest.NewPkgBuilder(config.State+"/pkg/test-1.0.0").
			WithName("test").
			WithBinaries("bin").
			WithVersion("1.0.0").
			WithEnvOps(
				&envars.Set{Name: "GOPATH", Value: config.Env + "/go"},
				&envars.Append{Name: "LD_LIBRARY_PATH", Value: config.Env + "/lib"},
				&envars.Prepend{Name: "PATH", Value: config.Env + "/bin"},
			).
			WithSource("www.example.com").
			Result(),
	}, {
		name: "Returns a manifest error for extra fields",
		files: map[string]string{
			"test.hcl": `
                description = ""
				binaries = ["bin"]
				root = "${version}/"
				foo = "bar"
				version "1.0.0" {
				  source = "www.example.com"
				}
            `,
		},
		manifestErrors: map[string][]string{
			"memory:///test.hcl": {"5:5: found extra fields \"foo\""},
		},
	}, {
		name: "Supports version matched channels with partial match",
		files: map[string]string{
			"test.hcl": `
                description = ""
				binaries = ["bin"]
				dest = "/test-${version}"

				version "1.0.0" { source = "www.example.com/00" }
				version "1.0.1" { source = "www.example.com/01" }
				version "1.1.0" { source = "www.example.com/11" }
				channel "testc" {
				  update = "5h"
				  version = "1.0.*"	
				}
            `,
		},
		reference: "test@testc",
		wantPkg: manifesttest.NewPkgBuilder("/test-1.0.1").
			WithName("test").
			WithBinaries("bin").
			WithChannel("testc").
			WithSource("www.example.com/01").
			WithDest("/test-1.0.1").
			WithUpdateInterval(5 * time.Hour).
			Result(),
	}, {
		name: "Supports version matched channels with any match",
		files: map[string]string{
			"test.hcl": `
                description = ""
				binaries = ["bin"]

				version "1.0.0" { source = "www.example.com/${version}" }
				version "1.0.1" { source = "www.example.com/${version}" }
				version "1.1.0" { source = "www.example.com/${version}" }
				channel "testc" {
				  update = "5h"
				  version = "*"	
				}
            `,
		},
		reference: "test@testc",
		wantPkg: manifesttest.NewPkgBuilder(config.State + "/pkg/test@testc").
			WithName("test").
			WithBinaries("bin").
			WithChannel("testc").
			WithSource("www.example.com/1.1.0").
			WithUpdateInterval(5 * time.Hour).
			Result(),
	}, {
		name: "Returns an error if channel version does not match anything",
		files: map[string]string{
			"test.hcl": `
                description = ""
				binaries = ["bin"]

				version "1.0.0" { source = "www.example.com/${version}" }
				version "1.0.1" { source = "www.example.com/${version}" }
				version "1.1.0" { source = "www.example.com/${version}" }
				channel "testc" {
				  update = "5h"
				  version = "2.0"	
				}
            `,
		},
		reference: "test@testc",
		manifestErrors: map[string][]string{
			"memory:///test.hcl": {"@testc: no version found matching 2.0"},
		},
		wantErr: "@testc: no version found matching 2.0",
	}, {
		name: "Returns unsupported core platforms",
		files: map[string]string{
			"test.hcl": `
                description = ""
				binaries = ["bin"]

				version "1.0.0" {
					linux { source = "www.example.com/${version}" }
				}
            `,
		},
		reference: "test-1.0.0",
		wantPkg: manifesttest.NewPkgBuilder(config.State + "/pkg/test-1.0.0").
			WithName("test").
			WithBinaries("bin").
			WithVersion("1.0.0").
			WithSource("www.example.com/1.0.0").
			WithUnsupportedPlatforms([]platform.Platform{{platform.Darwin, platform.Amd64}, {platform.Darwin, platform.Arm64}}).
			Result(),
	}, {
		name: "Validates event enum",
		files: map[string]string{
			"test.hcl": `
			description = ""
			binaries = ["bin"]

			on invalid {}

			version "1.0.0" {
				source = "www.example.com"
			}
			`,
		},
		reference: "test-1.0.0",
		wantErr:   `5:4: failed to unmarshal block: invalid label "event": invalid event "invalid"`,
		manifestErrors: map[string][]string{
			"memory:///test.hcl": {`5:4: failed to unmarshal block: invalid label "event": invalid event "invalid"`},
		},
	}, {
		name: "Local var interpolation",
		files: map[string]string{
			`test.hcl`: `
			description = ""
			binaries = ["bin"]

			source = "www.example.com/${path}"

			version "1.0.0" {
				vars = {
					path: "foo/bar",
				}
			}
			`,
		},
		reference: "test-1.0.0",
		wantPkg: manifesttest.NewPkgBuilder(config.State + "/pkg/test-1.0.0").
			WithName("test").
			WithBinaries("bin").
			WithVersion("1.0.0").
			WithSource("www.example.com/foo/bar").
			Result(),
	},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := ui.New(ui.LevelInfo, os.Stdout, os.Stderr, true, true)
			ss := []sources.Source{}
			for name, content := range tt.files {
				ss = append(ss, sources.NewMemSource(name, content))
			}
			l, err := New(sources.New("", ss), config)
			assert.NoError(t, err)
			if tt.reference != "" {
				gotPkg, err := l.Resolve(logger, PrefixSelector(ParseReference(tt.reference)))
				if err != nil || tt.wantErr != "" {
					assert.Equal(t, tt.wantErr, err.Error())
				}
				if gotPkg != nil {
					gotPkg.FS = nil
				}
				assert.Equal(t,
					repr.String(tt.wantPkg, repr.Indent("  "), repr.Hide[hcl.Position]()),
					repr.String(gotPkg, repr.Indent("  "), repr.Hide[hcl.Position]()))
			}
			wantErrors := tt.manifestErrors
			if wantErrors == nil {
				wantErrors = map[string][]string{}
			} else {
				err = l.LoadAll()
				assert.NoError(t, err)
			}
			errorMsgs := map[string][]string{}
			for k, errors := range l.Errors() {
				for _, e := range errors {
					errorMsgs[k] = append(errorMsgs[k], e.Error())
				}
			}
			assert.Equal(t, wantErrors, errorMsgs)
		})
	}
}

func TestSearchVersionsAndChannelsCoexist(t *testing.T) {
	files := map[string]string{
		"test.hcl": `
                description = ""
				binaries = ["bin"]
				version "1.0.0" {
				  source = "www.example.com"
				}
				channel stable {
				  source = "www.example.com"
				  update = "24h"
				}
				`,
	}
	config := Config{
		Env:   "/home/user/project",
		State: "/home/user/.cache/hermit",
		Platform: platform.Platform{
			OS:   "Linux",
			Arch: "x86_64",
		},
	}
	logger := ui.New(ui.LevelInfo, os.Stdout, os.Stderr, true, true)
	ffs := vfs.InMemoryFS(files)
	ss := []sources.Source{}
	for name, content := range files {
		ss = append(ss, sources.NewMemSource(name, content))
	}
	l, err := New(sources.New("", ss), config)
	assert.NoError(t, err)
	pkgs, err := l.Search(logger.Task("search"), "test")
	assert.NoError(t, err)
	expected := Packages{
		manifesttest.NewPkgBuilder(config.State + "/pkg/test@1").
			WithName("test").
			WithBinaries("bin").
			WithChannel("1").
			WithSource("www.example.com").
			WithUpdateInterval(time.Hour * 24).
			WithFS(ffs).
			Result(),
		manifesttest.NewPkgBuilder(config.State + "/pkg/test@1.0").
			WithName("test").
			WithBinaries("bin").
			WithChannel("1.0").
			WithSource("www.example.com").
			WithUpdateInterval(time.Hour * 24).
			WithFS(ffs).
			Result(),
		manifesttest.NewPkgBuilder(config.State + "/pkg/test@latest").
			WithName("test").
			WithBinaries("bin").
			WithChannel("latest").
			WithSource("www.example.com").
			WithUpdateInterval(time.Hour * 24).
			WithFS(ffs).
			Result(),
		manifesttest.NewPkgBuilder(config.State + "/pkg/test@stable").
			WithName("test").
			WithBinaries("bin").
			WithChannel("stable").
			WithSource("www.example.com").
			WithUpdateInterval(time.Hour * 24).
			WithFS(ffs).
			Result(),
		manifesttest.NewPkgBuilder(config.State + "/pkg/test-1.0.0").
			WithName("test").
			WithBinaries("bin").
			WithVersion("1.0.0").
			WithSource("www.example.com").
			WithFS(ffs).
			Result(),
	}
	assert.Equal(t, repr.String(expected, repr.Indent("  ")), repr.String(pkgs, repr.Indent("  ")))
}

func TestManifestImmutability(t *testing.T) {
	// Some data from the manifest is copied to newly created Packages. Some
	// of this data is modified (e.g., for environment variable expansion).
	// This test ensures that the original manifest is not modified. It
	// uses a smattering of expansions all over the place in different areas
	// to try to catch corner cases.
	files := map[string]string{
		"test.hcl": `
			description = "a package"
			binaries = ["bin-${os}"]
			env = { "FOO_ENV": "BAR_${version}" }
			vars = { "FOO_VAR": "something" }
			dest = "/tmp/foo/${version}"
			root = "/tmp/foo/${os}"
			source = "www.example.com/${version}/${os}/2"
			provides = ["bar-${version}"]
			on "unpack" {
				delete {
					files = ["/tmp/test-${os}"]
				}
			}

			version "1.0.0" "1.0.5" {
				source = "www.example.com/${version}/${os}"
				requires = ["foo-${os}"]
				on "activate" {
					chmod {
						file = "${root}/foo-${version}"
						mode = 493
					}
				}
			}

			platform "Linux" {
				mirrors = ["www.example.com/linux/${version}"]
				on "install" {
					mkdir {
						dir = "${FOO_VAR}"
					}
				}
			}
			platform "darwin" {
				mirrors = ["www.example.com/bsd/${version}"]
			}

			channel stable {
				source = "www.example.com"
				version = "*"
				update = "24h"
			}

			channel canary {
				source = "www.example.com/${os}-${arch}"
				update = "24h"
				version = "*"
				on "unpack" {
					rename {
						from = "/tmp/bar-${os}"
						to = "/tmp/test-canary-${version}"
					}
				}
			}
		`,
	}
	config := Config{
		Env:   "/home/user/project",
		State: "/home/user/.cache/hermit",
		Platform: platform.Platform{
			OS:   "Linux",
			Arch: "x86_64",
		},
	}
	ffs := vfs.InMemoryFS(files)
	mft, err := LoadManifestFile(ffs, "test.hcl")
	assert.NoError(t, err)

	mftCopy, err := LoadManifestFile(ffs, "test.hcl")
	assert.NoError(t, err)

	refs := mft.References("test")

	for _, ref := range refs {
		_, err := Resolve(mft, config, ref)
		assert.NoError(t, err)
		assert.Equal(t, mft, mftCopy)
	}
}
