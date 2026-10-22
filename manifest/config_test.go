package manifest

import (
	"os"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/alecthomas/repr"

	"github.com/alecthomas/hcl"

	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/platform"
	"github.com/cashapp/hermit/redact"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
)

func TestManifest(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		pkg      string
		os       string
		arch     string
		glibc    platform.GlibcVersion
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
		{name: "OSArchPlatformMatch",
			manifest: `
				binaries = ["bin/go"]
				description = "Go"
				platform "(darwin|linux)-amd64" {
					source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"
				}

				platform arm64 {
					source = "https://arm64-golang.org/dl/go${version}.${os}-${arch}.tar.gz"
				}

				version "1.14.4" {}
			`,
			os:   "linux",
			arch: "amd64",
			pkg:  "go-1.14.4",
			expected: &Package{
				Binaries: []string{"bin/go"},
				Source:   "https://golang.org/dl/go1.14.4.linux-amd64.tar.gz",
			},
		},
		{name: "GlibcPlatformNewerHost",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"
				platform linux {
					source = "https://golang.org/dl/go${version}.${os}-${arch}-musl.tar.gz"
				}
				platform linux glibc_2_17 {
					source = "https://golang.org/dl/go${version}.${os}-${arch}-glibc2.17.tar.gz"
				}
				platform linux glibc_2_28 {
					source = "https://golang.org/dl/go${version}.${os}-${arch}-glibc2.28.tar.gz"
				}
				version "1.14.4" {}
			`,
			os:    "linux",
			glibc: platform.GlibcVersion{Major: 2, Minor: 35},
			pkg:   "go-1.14.4",
			expected: &Package{
				Binaries: []string{"bin/go"},
				Source:   "https://golang.org/dl/go1.14.4.linux-amd64-glibc2.28.tar.gz",
			},
		},
		{name: "GlibcPlatformExactHost",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"
				platform linux {
					source = "https://golang.org/dl/go${version}.${os}-${arch}-musl.tar.gz"
				}
				platform linux glibc_2_17 {
					source = "https://golang.org/dl/go${version}.${os}-${arch}-glibc2.17.tar.gz"
				}
				platform linux glibc_2_28 {
					source = "https://golang.org/dl/go${version}.${os}-${arch}-glibc2.28.tar.gz"
				}
				version "1.14.4" {}
			`,
			os:    "linux",
			glibc: platform.GlibcVersion{Major: 2, Minor: 28},
			pkg:   "go-1.14.4",
			expected: &Package{
				Binaries: []string{"bin/go"},
				Source:   "https://golang.org/dl/go1.14.4.linux-amd64-glibc2.28.tar.gz",
			},
		},
		{name: "GlibcPlatformOlderHost",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"
				platform linux {
					source = "https://golang.org/dl/go${version}.${os}-${arch}-musl.tar.gz"
				}
				platform linux glibc_2_17 {
					source = "https://golang.org/dl/go${version}.${os}-${arch}-glibc2.17.tar.gz"
				}
				platform linux glibc_2_28 {
					source = "https://golang.org/dl/go${version}.${os}-${arch}-glibc2.28.tar.gz"
				}
				version "1.14.4" {}
			`,
			os:    "linux",
			glibc: platform.GlibcVersion{Major: 2, Minor: 27},
			pkg:   "go-1.14.4",
			expected: &Package{
				Binaries: []string{"bin/go"},
				Source:   "https://golang.org/dl/go1.14.4.linux-amd64-glibc2.17.tar.gz",
			},
		},
		{name: "GlibcPlatformNoGlibc",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"
				platform linux {
					source = "https://golang.org/dl/go${version}.${os}-${arch}-musl.tar.gz"
				}
				platform linux glibc_2_17 {
					source = "https://golang.org/dl/go${version}.${os}-${arch}-glibc2.17.tar.gz"
				}
				platform linux glibc_2_28 {
					source = "https://golang.org/dl/go${version}.${os}-${arch}-glibc2.28.tar.gz"
				}
				version "1.14.4" {}
			`,
			os:    "linux",
			glibc: platform.GlibcVersion{},
			pkg:   "go-1.14.4",
			expected: &Package{
				Binaries: []string{"bin/go"},
				Source:   "https://golang.org/dl/go1.14.4.linux-amd64-musl.tar.gz",
			},
		},
		{name: "GlibcOnlySourceIsSupported",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				darwin {
					source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"
				}
				platform linux amd64 glibc_2_17 {
					source = "https://golang.org/dl/go${version}.${os}-${arch}-glibc2.17.tar.gz"
				}
				version "1.14.4" {}
			`,
			os:    "linux",
			glibc: platform.GlibcVersion{Major: 2, Minor: 35},
			pkg:   "go-1.14.4",
			expected: &Package{
				Binaries: []string{"bin/go"},
				Source:   "https://golang.org/dl/go1.14.4.linux-amd64-glibc2.17.tar.gz",
			},
		},
		{name: "GlibcOnlySourceOnOlderHost",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				platform linux amd64 glibc_2_17 {
					source = "https://golang.org/dl/go${version}.${os}-${arch}-glibc2.17.tar.gz"
				}
				version "1.14.4" {}
			`,
			os:    "linux",
			glibc: platform.GlibcVersion{Major: 2, Minor: 12},
			pkg:   "go-1.14.4",
			fail:  "memory:///go.hcl: go-1.14.4 on os=linux arch=amd64 glibc=2.12: no source provided",
		},
		{name: "PlatformOverlay",
			manifest: `
				description = "Go"
				binaries = ["bin/go"]
				source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"

				platform linux amd64 {
					source = "https://amd64-linux-golang.org/dl/go${version}.${os}-${arch}.tar.gz"
				}

				platform linux arm {
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
			fail: "memory:///go.hcl: no version go-1.14.5 found in versions (go-1.14.4) or channels (go@1, go@1.14, go@latest), try \"hermit update\": unknown package",
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
				test.expected.Platform = platform.Platform{OS: hos, Arch: arch, Glibc: test.glibc}
				if test.expected.Files == nil {
					test.expected.Files = []*ResolvedFileRef{}
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
				Platform: platform.Platform{
					OS:    hos,
					Arch:  arch,
					Glibc: test.glibc,
				},
			})
			assert.NoError(t, err)
			pkg, err := resolver.Resolve(logger, ExactSelector(ParseReference(test.pkg)))
			if test.fail != "" {
				assert.EqualError(t, err, test.fail)
			} else {
				assert.NoError(t, err)
				test.expected.Root = "/tmp/hermit/pkg/" + test.pkg
				test.expected.Dest = "/tmp/hermit/pkg/" + test.pkg
				pkg.FS = nil
				assert.Equal(t,
					repr.String(test.expected, repr.Indent("  ")),
					repr.String(pkg, repr.Indent("  ")))
			}
		})
	}
}

// TODO: Currently not working because the structure is recursive. The HCL package should
//   (somehow?) support recursive schemas.
// func TestHelp(t *testing.T) {
// assert.Equal(t, ``, Help())
// }

func TestLayerSerialisesSourceURLsRaw(t *testing.T) {
	layer := Layer{
		Source:       redact.URL("https://user:sekret@example.com/pkg-${version}.tar.gz"),
		Mirrors:      []redact.URL{"https://user:sekret@mirror.example.com/pkg-${version}.tar.gz"},
		SHA256Source: redact.URL("https://user:sekret@example.com/pkg-${version}.sha256"),
	}
	data, err := hcl.Marshal(&layer)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "https://user:sekret@example.com/pkg-${version}.tar.gz")
	assert.Contains(t, string(data), "https://user:sekret@mirror.example.com/pkg-${version}.tar.gz")

	var back Layer
	assert.NoError(t, hcl.Unmarshal(data, &back))
	assert.Equal(t, layer.Source, back.Source)
	assert.Equal(t, layer.Mirrors, back.Mirrors)
	assert.Equal(t, layer.SHA256Source, back.SHA256Source)
}

func TestManifestPlatformVariants(t *testing.T) {
	m := &Manifest{}
	err := hcl.Unmarshal([]byte(`
		description = "Test"
		platform linux glibc_2_28 {}
		version "1.0.0" {
			platform linux amd64 glibc_2_17 {}
		}
		channel stable {
			update = "24h"
			platform glibc_2_28 {}
		}
	`), m)
	assert.NoError(t, err)
	variants, err := m.PlatformVariants(platform.Platforms{
		{OS: platform.Linux, Arch: platform.Amd64},
		{OS: platform.Darwin, Arch: platform.Arm64},
	})
	assert.NoError(t, err)
	assert.Equal(t, platform.Platforms{
		{OS: platform.Linux, Arch: platform.Amd64},
		{OS: platform.Linux, Arch: platform.Amd64, Glibc: platform.GlibcVersion{Major: 2, Minor: 17}},
		{OS: platform.Linux, Arch: platform.Amd64, Glibc: platform.GlibcVersion{Major: 2, Minor: 28}},
		{OS: platform.Darwin, Arch: platform.Arm64},
	}, variants)
}

func TestManifestPlatformVariantsInvalid(t *testing.T) {
	m := &Manifest{}
	err := hcl.Unmarshal([]byte(`
		description = "Test"
		platform linux glibc_2_x {}
	`), m)
	assert.NoError(t, err)
	_, err = m.PlatformVariants(platform.Core)
	assert.EqualError(t, err, `invalid platform label "glibc_2_x", expected glibc_<major>_<minor>`)
}
