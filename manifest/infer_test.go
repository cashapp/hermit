package manifest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/redact"
	"github.com/cashapp/hermit/ui"
)

func TestInfer(t *testing.T) {
	files := map[string]string{
		"/releases/download/0.1.1/pkg-0.1.1-darwin-amd64.zip": "",
		"/releases/download/0.1.1/pkg-0.1.1-linux-amd64.tgz":  "",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, ok := files[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, content)
	}))
	defer srv.Close()
	p, _ := ui.NewForTesting()
	actual, err := InferFromArtefact(
		p,
		cache.GetSource,
		http.DefaultClient,
		github.New(nil, nil),
		srv.URL+"/releases/download/0.1.1/pkg-0.1.1-linux-amd64.tgz",
		"",
	)
	assert.NoError(t, err)
	expected := &Manifest{
		Layer: Layer{
			Binaries: []string{},
			Platform: []*PlatformBlock{
				{Attrs: []string{"darwin", "amd64"}, Layer: Layer{Source: redact.URL(srv.URL + "/releases/download/${version}/pkg-${version}-${os}-${arch}.zip")}},
				{Attrs: []string{"darwin", "arm64"}, Layer: Layer{Source: redact.URL(srv.URL + "/releases/download/${version}/pkg-${version}-${os}-amd64.zip")}},
				{Attrs: []string{"linux", "amd64"}, Layer: Layer{Source: redact.URL(srv.URL + "/releases/download/${version}/pkg-${version}-${os}-${arch}.tgz")}},
			},
		},
		Versions: []VersionBlock{{
			Version: []string{"0.1.1"},
			AutoVersion: &AutoVersionBlock{
				GitHubRelease:  "",
				VersionPattern: "v?(.*)",
			},
		}},
	}
	assert.Equal(t, expected, actual)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestInferFromArtefactUsesGHEHostForRepoLookup(t *testing.T) {
	var hosts []string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		hosts = append(hosts, req.URL.Host)
		assert.NotEqual(t, "github.com", req.URL.Host)
		assert.NotEqual(t, "api.github.com", req.URL.Host)

		switch {
		case req.Method == http.MethodGet && req.URL.Host == "api.mycompany.ghe.com" && req.URL.Path == "/repos/mycompany/tool":
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"description":"ghe repo","homepage":"https://example.com"}`)),
				Request:    req,
			}, nil
		case req.Method == http.MethodHead && req.URL.Host == "mycompany.ghe.com":
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		default:
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		}
	})}

	p, _ := ui.NewForTesting()
	actual, err := InferFromArtefact(
		p,
		cache.GetSource,
		client,
		github.New(client, []github.HostConfig{{WebHost: "github.com"}, {WebHost: "mycompany.ghe.com", Token: "enterprise-token"}}),
		"https://mycompany.ghe.com/mycompany/tool/releases/download/v1.2.3/tool-v1.2.3-linux-amd64.tgz",
		"",
	)
	assert.NoError(t, err)
	assert.Equal(t, "ghe repo", actual.Description)
	assert.Equal(t, "https://example.com", actual.Homepage)
	assert.Equal(t, "", actual.Versions[0].AutoVersion.GitHubRelease)
	assert.True(t, slices.Contains(hosts, "api.mycompany.ghe.com"))
}
