package manifest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cashapp/hermit/github"
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
	actual, err := InferFromArtefact(p, http.DefaultClient, github.New(""), srv.URL+"/releases/download/0.1.1/pkg-0.1.1-linux-amd64.tgz", "")
	require.NoError(t, err)
	expected := &Manifest{
		Layer: Layer{
			Binaries: []string{},
			Platform: []*PlatformBlock{
				{Attrs: []string{"darwin", "amd64"}, Layer: Layer{Source: srv.URL + "/releases/download/${version}/pkg-${version}-${os}-${arch}.zip"}},
				{Attrs: []string{"darwin", "arm64"}, Layer: Layer{Source: srv.URL + "/releases/download/${version}/pkg-${version}-${os}-amd64.zip"}},
				{Attrs: []string{"linux", "amd64"}, Layer: Layer{Source: srv.URL + "/releases/download/${version}/pkg-${version}-${os}-${arch}.tgz"}},
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
	require.Equal(t, expected, actual)
}
