package manifest

import (
	"os"
	"testing"

	"github.com/cashapp/hermit/sources"

	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/ui"

	"github.com/alecthomas/repr"
	"github.com/stretchr/testify/require"
)

func TestManifest(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		pkg      string
		os       string
		arch     string
		expected *Package
		fail     string
	}{
		{name: "MultiVersionBlockSelectFirst",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"
				version "1.13.5" "1.14.4" {}
		`,
			pkg: `go-1.13.5`,
			expected: &Package{
				Binaries:  []string{"bin/go"},
				Reference: ParseReference("go-1.13.5"),
				Source:    "https://golang.org/dl/go1.13.5.darwin-amd64.tar.gz",
			},
		},
		{name: "MultiVersionBlockSelectSecond",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"
				version "1.13.5" "1.14.4" {}
		`,
			pkg: `go-1.14.4`,
			expected: &Package{
				Binaries:  []string{"bin/go"},
				Reference: ParseReference("go-1.14.4"),
				Source:    "https://golang.org/dl/go1.14.4.darwin-amd64.tar.gz",
			},
		},
		{name: "Go",
			manifest: `
				description = "Go"
				env = {
				  GOROOT: "${root}"
				}
				binaries = ["bin/go"]
				source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"

				version "1.13.5" {}
				version "1.14.4" {}
				`,
			pkg: "go-1.14.4",
			expected: &Package{
				Binaries:  []string{"bin/go"},
				Reference: ParseReference("go-1.14.4"),
				Env: []envars.Op{
					&envars.Set{"GOROOT", "/tmp/hermit/pkg/go-1.14.4"},
				},
				Source: "https://golang.org/dl/go1.14.4.darwin-amd64.tar.gz",
			},
		},
		{name: "OSOverlay",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"

				linux {
					source = "https://linux-golang.org/dl/go${version}.${os}-${arch}.tar.gz"
				}

				version "1.14.4" {}
			`,
			os:  "linux",
			pkg: "go-1.14.4",
			expected: &Package{
				Reference: ParseReference("go-1.14.4"),
				Binaries:  []string{"bin/go"},
				Source:    "https://linux-golang.org/dl/go1.14.4.linux-amd64.tar.gz",
			},
		},
		{name: "OSArchOverlay",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"

				linux {
					arch = "amd64"
					source = "https://amd64-linux-golang.org/dl/go${version}.${os}-${arch}.tar.gz"
				}

				linux {
					arch = "arm"
					source = "https://arm-linux-golang.org/dl/go${version}.${os}-${arch}.tar.gz"
				}

				version "1.14.4" {}
			`,
			os:   "linux",
			arch: "arm",
			pkg:  "go-1.14.4",
			expected: &Package{
				Arch:      "arm",
				Reference: ParseReference("go-1.14.4"),
				Binaries:  []string{"bin/go"},
				Source:    "https://arm-linux-golang.org/dl/go1.14.4.linux-arm.tar.gz",
			},
		},
		{name: "VersionOverlay",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"

				version "1.14.4" {
					source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"
				}
			`,
			os:  "linux",
			pkg: "go-1.14.4",
			expected: &Package{
				Root:      "/tmp/hermit/go-1.14.4",
				Reference: ParseReference("go-1.14.4"),
				Binaries:  []string{"bin/go"},
				Source:    "https://golang.org/dl/go1.14.4.linux-amd64.tar.gz",
			},
		},
		{name: "Binaries",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"

				version "1.14.4" {}
			`,
			pkg: "go-1.14.4",
			expected: &Package{
				Reference: ParseReference("go-1.14.4"),
				Binaries:  []string{"bin/go"},
				Source:    "https://golang.org/dl/go1.14.4.darwin-amd64.tar.gz",
			},
		},
		{name: "InvalidVersion",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"

				version "1.14.4" {}
			`,
			pkg:  "go-1.14.5",
			fail: "memory:///go.hcl: no version go-1.14.5 in known versions 1.14.4, @1, @1.14, @latest: unknown package",
		},
		{name: "InvalidVariable",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				source = "https://golang.org/dl/go${PK_VERSION}.${os}-${arch}.tar.gz"

				version "1.14.4" {}
			`,
			pkg:  "go-1.14.4",
			fail: "unknown variable $PK_VERSION",
		},
		{name: "DeferredEnvars",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"
				env = {
					GOBIN: "${HERMIT_ENV}/build",
					PATH: "${GOBIN}:${PATH}"
				}
				version "1.14.4" {}
			`,
			pkg: "go-1.14.4",
			expected: &Package{
				Env: []envars.Op{
					&envars.Set{"GOBIN", "/project/build"},
					&envars.Prepend{"PATH", "${GOBIN}"},
				},
				Reference: ParseReference("go-1.14.4"),
				Binaries:  []string{"bin/go"},
				Source:    "https://golang.org/dl/go1.14.4.darwin-amd64.tar.gz",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hos := "darwin"
			if test.os != "" {
				hos = test.os
			}
			arch := "amd64"
			if test.arch != "" {
				arch = test.arch
			}
			if test.expected != nil {
				test.expected.Description = "Go"
				if test.expected.Files == nil {
					test.expected.Files = []*ResolvedFileRef{}
				}
				if test.expected.Rename == nil {
					test.expected.Rename = map[string]string{}
				}
				if !test.expected.Reference.IsFullyQualified() {
					test.expected.Reference = ParseReference(test.pkg)
				}
				if test.expected.Root == "" {
					test.expected.Root = "/tmp/hermit/" + test.expected.Reference.String()
				}
				if test.expected.Triggers == nil {
					test.expected.Triggers = map[Event][]Action{}
				}
			}
			logger := ui.New(ui.LevelInfo, os.Stdout, os.Stderr, true, true)
			resolver, err := New(sources.New("", []sources.Source{
				sources.NewMemSource("go.hcl", test.manifest),
			}), Config{
				Env:   "/project",
				State: "/tmp/hermit",
				OS:    hos,
				Arch:  arch,
			})
			require.NoError(t, err)
			pkg, err := resolver.Resolve(logger, ExactSelector(ParseReference(test.pkg)))
			if test.fail != "" {
				require.EqualError(t, err, test.fail)
			} else {
				require.NoError(t, err)
				test.expected.Root = "/tmp/hermit/pkg/" + test.pkg
				test.expected.Dest = "/tmp/hermit/pkg/" + test.pkg
				pkg.FS = nil
				require.Equal(t,
					repr.String(test.expected, repr.Indent("  ")),
					repr.String(pkg, repr.Indent("  ")))
			}
		})
	}
}

// TODO: Currently not working because the structure is recursive. The HCL package should
//   (somehow?) support recursive schemas.
// func TestHelp(t *testing.T) {
// require.Equal(t, ``, Help())
// }
