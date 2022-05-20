package app

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cashapp/hermit/hermittest"
	"github.com/cashapp/hermit/ui"
)

// test that search results are correctly sorted by prefix then name
func TestSearchPrefixSortsResults(t *testing.T) {
	l, buf := ui.NewForTesting()
	// l.SetProgressBarEnabled(false)
	// l.SetLevel(ui.LevelDebug)
	f := hermittest.NewEnvTestFixture(t, nil)
	f.WithManifests(
		map[string]string{
			"test.hcl": `
					description = ""
					binaries = ["bin1"]
					version "1.0.0" {
					  source = "https://example.com/test"
					}
				`,
			"attest.hcl": `
					description = ""
					binaries = ["bin1"]
					version "1.0.0" {
						source = "https://example.com/test"
					}
				`,
			"untested.hcl": `
					description = ""
					binaries = ["bin1"]
					version "1.0.0" {
						source = "https://example.com/test"
					}
				`,
			"unavailable.hcl": `
					description = ""
					binaries = ["bin1"]
					version "1.0.0" {
						source = "https://example.com/test"
					}
			`,
		},
	)
	availablePkgs, err := f.Env.Search(l, "test")
	require.NoError(t, err)
	defer f.Clean()

	require.NoError(t, listPackages(availablePkgs, &listPackageOption{
		AllVersions:   true,
		TransformJSON: buildSearchJSONResults,
		UI:            l,
		JSON:          false,
		Prefix:        "test",
	}))

	bufStr := buf.String()
	lines := strings.Split(bufStr, "\n")
	t.Log(lines)
	t.Log(len(lines))

	// require.NoError(t, err)
	require.Equal(t, 3, len(lines))
	require.Equal(t, 4, len(lines))
	// correct order is test -> attest -> untested
	// require.Equal(t, "test", pkgs[0].Reference.Name)
	// require.Equal(t, "attest", pkgs[1].Reference.Name)
	// require.Equal(t, "untested", pkgs[2].Reference.Name)
}
