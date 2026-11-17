package app

import (
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/cashapp/hermit/manifest"
)

func TestParseToolVersions(t *testing.T) {
	input := `
# Pinned for CI
nodejs 20.11.0
golang   1.22.1   # trailing comment
python 3.12.2 3.11.8

	terraform	1.7.4
`
	entries, err := parseToolVersions(strings.NewReader(input))
	assert.NoError(t, err)
	assert.Equal(t, []toolVersion{
		{Line: 3, Tool: "nodejs", Versions: []string{"20.11.0"}},
		{Line: 4, Tool: "golang", Versions: []string{"1.22.1"}},
		{Line: 5, Tool: "python", Versions: []string{"3.12.2", "3.11.8"}},
		{Line: 7, Tool: "terraform", Versions: []string{"1.7.4"}},
	}, entries)
}

func TestParseToolVersionsMissingVersion(t *testing.T) {
	_, err := parseToolVersions(strings.NewReader("nodejs 20.11.0\nrust\n"))
	assert.EqualError(t, err, `.tool-versions:2: "rust" has no version`)
}

func TestToolVersionSelectors(t *testing.T) {
	tests := []struct {
		tool     string
		version  string
		expected []string
		err      string
	}{
		{tool: "nodejs", version: "20.11.0", expected: []string{"node-20.11.0", "node-20.11.0.*"}},
		{tool: "node", version: "20", expected: []string{"node-20", "node-20.*"}},
		{tool: "golang", version: "1.22.1", expected: []string{"go-1.22.1", "go-1.22.1.*"}},
		{tool: "python", version: "3.12.2", expected: []string{"python3-3.12.2", "python3-3.12.2.*"}},
		{tool: "terraform", version: "v1.7.4", expected: []string{"terraform-1.7.4", "terraform-1.7.4.*"}},
		{tool: "nodejs", version: "latest", expected: []string{"node"}},
		{tool: "nodejs", version: "latest:20", expected: []string{"node-20.*"}},
		{tool: "java", version: "temurin-17.0.2+8", expected: []string{"openjdk-17.0.2_8", "openjdk-17.0.2_8.*"}},
		{tool: "java", version: "adoptopenjdk-11.0.10+9", expected: []string{"openjdk-11.0.10_9", "openjdk-11.0.10_9.*"}},
		{tool: "java", version: "corretto-21.0.5.11.1", expected: []string{"corretto-21.0.5.11.1", "corretto-21.0.5.11.1.*"}},
		{tool: "java", version: "21", expected: []string{"openjdk-21", "openjdk-21.*"}},
		{tool: "java", version: "zulu-17.0.2", err: `Java distribution "zulu" has no Hermit equivalent`},
		{tool: "nodejs", version: "system", err: "uses the system installation"},
		{tool: "nodejs", version: "lts", err: `version "lts" is not supported`},
		{tool: "ruby", version: "ref:main", err: `version "ref:main" is not supported`},
		{tool: "ruby", version: "path:/opt/ruby", err: `version "path:/opt/ruby" is not supported`},
	}
	for _, test := range tests {
		t.Run(test.tool+"@"+test.version, func(t *testing.T) {
			selectors, err := toolVersionSelectors(test.tool, test.version)
			if test.err != "" {
				assert.EqualError(t, err, test.err)
				return
			}
			assert.NoError(t, err)
			actual := make([]string, 0, len(selectors))
			for _, selector := range selectors {
				actual = append(actual, selector.String())
			}
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestToolVersionSelectorsMatching(t *testing.T) {
	ref := manifest.ParseReference

	selectors, err := toolVersionSelectors("nodejs", "20.11")
	assert.NoError(t, err)
	exact, prefix := selectors[0], selectors[1]
	assert.False(t, exact.Matches(ref("node-20.11.1")))
	assert.True(t, prefix.Matches(ref("node-20.11.1")))
	// "20.1" must not match "20.11.0".
	selectors, err = toolVersionSelectors("nodejs", "20.1")
	assert.NoError(t, err)
	assert.False(t, selectors[1].Matches(ref("node-20.11.0")))
	assert.True(t, selectors[1].Matches(ref("node-20.1.0")))

	// Glob metacharacters in versions are matched literally.
	selectors, err = toolVersionSelectors("java", "temurin-17.0.2+8")
	assert.NoError(t, err)
	assert.True(t, selectors[0].Matches(ref("openjdk-17.0.2_8")))
	assert.False(t, selectors[0].Matches(ref("openjdk-17.0.2_9")))
}

func TestExactSelector(t *testing.T) {
	sel, err := exactSelector(manifest.ParseReference("node-20.11.0"))
	assert.NoError(t, err)
	assert.True(t, sel.Matches(manifest.ParseReference("node-20.11.0")))
	assert.False(t, sel.Matches(manifest.ParseReference("node-20.11.1")))

	sel, err = exactSelector(manifest.ParseReference("corretto@stable"))
	assert.NoError(t, err)
	assert.True(t, sel.Matches(manifest.ParseReference("corretto@stable")))
	assert.False(t, sel.Matches(manifest.ParseReference("corretto-21.0.5.11.1")))
}
