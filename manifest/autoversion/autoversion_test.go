package autoversion

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/github"
)

type testGHAPI []string

func (v testGHAPI) LatestRelease(repo string) (*github.Release, error) {
	return &github.Release{TagName: v[0]}, nil
}

func (v testGHAPI) Releases(repo string, limit int) (releases []*github.Release, err error) {
	for i := 0; i < limit && i < len(v); i++ {
		releases = append(releases, &github.Release{TagName: v[i]})
	}
	return
}

type testHTTPClient struct {
	path string
}

func (t testHTTPClient) RoundTrip(req *http.Request) (*http.Response, error) {
	r, err := os.Open(t.path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &http.Response{
		StatusCode: 200,
		Body:       r,
	}, nil
}

func TestAutoVersion(t *testing.T) {
	expectedAdded := map[string][]string{
		"css":             {"0.11.0"},
		"github":          {"3.2.150", "5.1.234"},
		"html":            {"1.17.5"},
		"htmlattr":        {"0.1.12"},
		"json":            {"1.2.3"},
		"multiple_blocks": {"1.16.12", "1.17.5"},
	}
	inputs, err := filepath.Glob("testdata/*.input.hcl")
	assert.NoError(t, err)
	for _, input := range inputs {
		name := strings.TrimSuffix(filepath.Base(input), ".input.hcl")
		t.Run(strings.Title(name), func(t *testing.T) {
			// Copy input manifest to a temporary path.
			tmpFile, err := os.CreateTemp(t.TempDir(), "*.hcl")
			assert.NoError(t, err)
			defer tmpFile.Close()

			inputContent, err := os.ReadFile(input)
			assert.NoError(t, err)

			_, err = io.Copy(tmpFile, bytes.NewReader(inputContent))
			assert.NoError(t, err)
			tmpFile.Close()

			ghClient := testGHAPI([]string{"v3.2.150", "v5.1.234"})
			var hClient *http.Client
			httpInput := strings.ReplaceAll(input, ".input.hcl", ".http")
			if _, err := os.Stat(httpInput); err == nil {
				hClient = &http.Client{
					Transport: testHTTPClient{httpInput},
				}
			}

			added, err := AutoVersion(hClient, ghClient, tmpFile.Name())
			assert.NoError(t, err)
			assert.Equal(t, expectedAdded[name], added)

			actualContent, err := os.ReadFile(tmpFile.Name())
			assert.NoError(t, err)

			expectedContent, err := os.ReadFile(strings.ReplaceAll(input, ".input.", ".expected."))
			assert.NoError(t, err)

			assert.Equal(t, string(expectedContent), string(actualContent))
		})
	}
}
