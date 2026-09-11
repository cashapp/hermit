package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/hermittest"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/ui"
)

// Test that the json output from info command matches what is expected in the IJ plugin
func TestCommandInfoJson(t *testing.T) {
	l, buf := ui.NewForTesting()
	f := hermittest.NewEnvTestFixture(t, nil).WithManifests(map[string]string{
		"test-version.hcl": `
			description = "test package"
			binaries = ["bin"]
			version "1.1" {
			  source = "www.example.com"
			}
		`,
	})
	defer f.Clean()

	cmd := infoCmd{
		Packages: []manifest.GlobSelector{manifest.MustParseGlobSelector("test-version-1.1")},
		JSONFormattable: JSONFormattable{
			JSON: true,
		},
	}
	assert.NoError(t, cmd.Run(l, f.Env, f.State))

	var jss []map[string]json.RawMessage
	assert.NoError(t, json.Unmarshal(buf.Bytes(), &jss))
	assert.Equal(t, 1, len(jss))
	js := jss[0]

	assert.Equal(t, stringValue(t, js["Description"]), "test package")
	assert.Equal(t, stringValue(t, js["Reference"], "Name"), "test-version")
	assert.Equal(t, stringValue(t, js["Reference"], "Version"), "1.1")
	assert.Equal(t, stringValue(t, js["Root"]), f.State.PkgDir()+"/test-version-1.1")
	// The package isn't installed, so its binaries can't be resolved and are left as-is.
	assert.Equal(t, []string{"bin"}, stringsValue(t, js["Binaries"]))
}

func TestCommandInfoJsonExpandsBinaryGlobs(t *testing.T) {
	l, buf := ui.NewForTesting()
	f := hermittest.NewEnvTestFixture(t, nil).WithManifests(map[string]string{
		"test-globs.hcl": `
			description = "test package"
			binaries = ["bin/*"]
			version "1.0" {
			  source = "www.example.com"
			}
		`,
	})
	defer f.Clean()

	binDir := filepath.Join(f.State.PkgDir(), "test-globs-1.0", "bin")
	assert.NoError(t, os.MkdirAll(binDir, 0700))
	for _, name := range []string{"foo", "bar"} {
		assert.NoError(t, os.WriteFile(filepath.Join(binDir, name), nil, 0600))
	}

	cmd := infoCmd{
		Packages: []manifest.GlobSelector{manifest.MustParseGlobSelector("test-globs-1.0")},
		JSONFormattable: JSONFormattable{
			JSON: true,
		},
	}
	assert.NoError(t, cmd.Run(l, f.Env, f.State))

	var jss []map[string]json.RawMessage
	assert.NoError(t, json.Unmarshal(buf.Bytes(), &jss))
	assert.Equal(t, 1, len(jss))
	assert.Equal(t, []string{"bin/bar", "bin/foo"}, stringsValue(t, jss[0]["Binaries"]))
}

func stringsValue(t *testing.T, from json.RawMessage) []string {
	t.Helper()
	var res []string
	assert.NoError(t, json.Unmarshal(from, &res))
	return res
}

func stringValue(t *testing.T, from json.RawMessage, path ...string) string {
	t.Helper()
	if len(path) == 0 {
		res := ""
		assert.NoError(t, json.Unmarshal(from, &res))
		return res
	}
	var jss map[string]json.RawMessage
	assert.NoError(t, json.Unmarshal(from, &jss))
	return stringValue(t, jss[path[0]], path[1:]...)
}
