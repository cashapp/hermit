package autoversion

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/github"
)

type testGHAPI struct{}

func (v testGHAPI) LatestRelease(repo string) (*github.Release, error) {
	return &github.Release{TagName: "v3.2.150"}, nil
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
	inputs, err := filepath.Glob("testdata/*.input.hcl")
	require.NoError(t, err)
	for _, input := range inputs {
		t.Run(strings.Title(strings.TrimSuffix(filepath.Base(input), ".input.hcl")), func(t *testing.T) {
			// Copy input manifest to a temporary path.
			tmpFile, err := os.CreateTemp("", "*.hcl")
			require.NoError(t, err)
			defer tmpFile.Close() // nolint
			defer os.Remove(tmpFile.Name())

			inputContent, err := os.ReadFile(input)
			require.NoError(t, err)

			_, err = io.Copy(tmpFile, bytes.NewReader(inputContent))
			require.NoError(t, err)
			tmpFile.Close()

			ghClient := testGHAPI{}
			var hClient *http.Client
			httpInput := strings.ReplaceAll(input, ".input.hcl", ".http")
			if _, err := os.Stat(httpInput); err == nil {
				hClient = &http.Client{
					Transport: testHTTPClient{httpInput},
				}
			}

			_, err = AutoVersion(hClient, ghClient, tmpFile.Name())
			require.NoError(t, err)

			actualContent, err := os.ReadFile(tmpFile.Name())
			require.NoError(t, err)

			expectedContent, err := os.ReadFile(strings.ReplaceAll(input, ".input.", ".expected."))
			require.NoError(t, err)

			require.Equal(t, string(expectedContent), string(actualContent))
		})
	}
}
