package digest

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/hermittest"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/redact"
	"github.com/cashapp/hermit/ui"
)

func TestTryGetSHADoesNotLeakURLCredentials(t *testing.T) {
	u, buf := ui.NewForTesting()
	task := u.Task("test")
	pkg := &manifest.Package{
		Source:       redact.URL("https://sekret-token@127.0.0.1:1/owner/file.tar.gz"),
		SHA256Source: redact.URL("https://sekret-token@127.0.0.1:1/owner/file.sha256"),
	}

	digest := tryGetSHA(task, http.DefaultClient, pkg)

	assert.Equal(t, "", digest)
	assert.NotContains(t, buf.String(), "sekret-token")
	assert.Contains(t, buf.String(), "https://127.0.0.1:1/owner/file.sha256")
}

func TestUpdateDigestsSkipsUnsupportedPlatforms(t *testing.T) {
	var (
		lock     sync.Mutex
		requests []string
	)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lock.Lock()
		requests = append(requests, r.URL.Path)
		lock.Unlock()
		if !strings.HasSuffix(r.URL.Path, ".tar.gz") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(r.URL.Path))
	})
	fixture := hermittest.NewEnvTestFixture(t, handler)

	path := filepath.Join(t.TempDir(), "test.hcl")
	err := os.WriteFile(path, []byte(fmt.Sprintf(`
description = "Test"
binaries = ["bin"]
source = "%s/test-${version}-${os}-${arch}.tar.gz"

version "1.0.0" {
  platform linux-arm64 { unsupported = true }
}
`, fixture.Server.URL)), 0600)
	assert.NoError(t, err)

	err = UpdateDigests(fixture.P, fixture.Server.Client(), fixture.State, path)
	assert.NoError(t, err)

	for _, request := range requests {
		assert.NotContains(t, request, "linux-arm64")
	}
	mani, err := manifest.LoadManifestFile(os.DirFS(filepath.Dir(path)), filepath.Base(path))
	assert.NoError(t, err)
	assert.Equal(t, 3, len(mani.SHA256Sums))
}
