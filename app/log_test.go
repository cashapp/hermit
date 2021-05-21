package app

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/acarl005/stripansi"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/hermittest"
	"github.com/cashapp/hermit/ui"
	"github.com/stretchr/testify/require"
)

func TestLogs(t *testing.T) {
	tests := []struct {
		name string
		fn   func(*ui.UI, *hermittest.EnvTestFixture)
		tmpl string
	}{{
		name: "install",
		fn: func(l *ui.UI, f *hermittest.EnvTestFixture) {
			cmd := installCmd{Packages: []string{"tpkg-0.9.0"}}
			err := cmd.Run(l, f.Env, f.State)
			require.NoError(t, err)
		},
		tmpl: `
			info:tpkg-0.9.0:install: Installing tpkg-0.9.0
			debug:tpkg-0.9.0:install: From {{.Source}}
			debug:tpkg-0.9.0:install: To {{.State}}/pkg/tpkg-0.9.0
			debug:tpkg-0.9.0:download: Downloading {{.Source}}
			debug:tpkg-0.9.0:unpack: Extracting {{.Cache}} to {{.State}}/pkg/tpkg-0.9.0
			debug:tpkg-0.9.0:link: Linking binaries for tpkg-0.9.0
			debug:tpkg-0.9.0:link: ln -s "hermit" "{{.Bin}}/.tpkg-0.9.0.pkg"
			debug:tpkg-0.9.0:link: ln -s ".tpkg-0.9.0.pkg" "{{.Bin}}/darwin_exe"`,
	}, {
		name: "upgrade",
		fn: func(l *ui.UI, f *hermittest.EnvTestFixture) {
			cmd := upgradeCmd{Packages: nil}
			err := cmd.Run(l, f.Env)
			require.NoError(t, err)
		},
		tmpl: `
			info:tpkg:upgrade: Upgrading tpkg-0.9.0 to tpkg-0.10.0
			info:tpkg-0.9.0:uninstall: Uninstalling tpkg-0.9.0
			debug:tpkg-0.9.0:unlink: Uninstalling tpkg-0.9.0
			info:tpkg-0.10.0:install: Installing tpkg-0.10.0
			debug:tpkg-0.10.0:install: From {{.Source}}
			debug:tpkg-0.10.0:install: To {{.State}}/pkg/tpkg-0.10.0
			debug:tpkg-0.10.0:unpack: Extracting {{.Cache}} to {{.State}}/pkg/tpkg-0.10.0
			debug:tpkg-0.10.0:link: Linking binaries for tpkg-0.10.0
			debug:tpkg-0.10.0:link: ln -s "hermit" "{{.Bin}}/.tpkg-0.10.0.pkg"
			debug:tpkg-0.10.0:link: ln -s ".tpkg-0.10.0.pkg" "{{.Bin}}/darwin_exe"`,
	}, {
		name: "gc",
		fn: func(l *ui.UI, f *hermittest.EnvTestFixture) {
			cmd := gcCmd{}
			err := cmd.Run(l, f.Env)
			require.NoError(t, err)
		},
		tmpl: `
			debug: rm -rf "{{.State}}/cache"
			info:tpkg-0.9.0: Clearing tpkg-0.9.0
			debug:tpkg-0.9.0:remove: chmod -R +w {{.State}}/pkg/tpkg-0.9.0
			debug:tpkg-0.9.0:remove: rm -rf {{.State}}/pkg/tpkg-0.9.0`,
	}, {
		name: "uninstall",
		fn: func(l *ui.UI, f *hermittest.EnvTestFixture) {
			cmd := uninstallCmd{Packages: []string{"tpkg"}}
			err := cmd.Run(l, f.Env)
			require.NoError(t, err)
		},
		tmpl: `
			info:tpkg-0.10.0:uninstall: Uninstalling tpkg-0.10.0
			debug:tpkg-0.10.0:unlink: Uninstalling tpkg-0.10.0
			`,
	}}

	handler := staticFileHTTPHandler(t, "../archive/testdata")

	f := hermittest.NewEnvTestFixture(t, handler)
	f.WithManifests(map[string]string{
		"tpkg.hcl": `
			description = ""
			binaries = ["darwin_exe"]
			version "0.9.0" {
			  source = "` + f.Server.URL + `/archive.tar.gz"
			}
			version "0.10.0" {
			  source = "` + f.Server.URL + `/archive.tar.gz"
			}
		`,
	})
	defer f.Clean()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testLogsFor(t, f, tt.fn, tt.tmpl)
		})
	}
}

func testLogsFor(
	t *testing.T,
	f *hermittest.EnvTestFixture,
	test func(*ui.UI, *hermittest.EnvTestFixture),
	tmpl string) {

	t.Helper()

	l, buf := ui.NewForTesting()
	l.SetProgressBarEnabled(false)
	l.SetLevel(ui.LevelDebug)

	type state struct {
		Source string
		Cache  string
		State  string
		Env    string
		Bin    string
	}

	trimmed := strings.TrimSpace(trimLines(tmpl))
	expected, err := template.New("expected").Parse(trimmed)
	require.NoError(t, err)

	tbuf := bytes.Buffer{}
	uri := f.Server.URL + "/archive.tar.gz"
	err = expected.Execute(&tbuf, state{
		Source: uri,
		State:  f.State.Root(),
		Cache:  filepath.Join(f.State.Root(), "cache", cache.BasePath("", uri)),
		Env:    f.Env.EnvDir(),
		Bin:    f.Env.BinDir(),
	})
	require.NoError(t, err)

	test(l, f)

	log := buf.String()
	log = stripansi.Strip(log)

	require.Equal(t, tbuf.String(), strings.TrimSpace(log))
}

func trimLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.Join(lines, "\n")
}

func staticFileHTTPHandler(t *testing.T, dir string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("ETag", "testtag")
		dat, err := ioutil.ReadFile(filepath.Join(dir, r.RequestURI))
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			_, err := w.Write(dat)
			require.NoError(t, err)
		}
	}
}
