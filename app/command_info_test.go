package app

import (
	"encoding/json"
	"github.com/cashapp/hermit/hermittest"
	"github.com/cashapp/hermit/ui"
	"github.com/stretchr/testify/require"
	"testing"
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

	cmd := infoCmd{Packages: []string{"test-version-1.1"}, JSON: true}
	require.NoError(t, cmd.Run(l, f.Env, f.State))

	var jss []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(buf.String()), &jss))
	require.Equal(t, 1, len(jss))
	js := jss[0]

	require.Equal(t, stringValue(t, js["Description"]), "test package")
	require.Equal(t, stringValue(t, js["Reference"], "Name"), "test-version")
	require.Equal(t, stringValue(t, js["Reference"], "Version"), "1.1")
	require.Equal(t, stringValue(t, js["Root"]), f.State.PkgDir()+"/test-version-1.1")
}

func stringValue(t *testing.T, from json.RawMessage, path ...string) string {
	t.Helper()
	if len(path) == 0 {
		res := ""
		require.NoError(t, json.Unmarshal(from, &res))
		return res
	}
	var jss map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(from, &jss))
	return stringValue(t, jss[path[0]], path[1:]...)
}
