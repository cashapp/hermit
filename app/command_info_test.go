package app

import (
	"encoding/json"
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
