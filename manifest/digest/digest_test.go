package digest

import (
	"net/http"
	"testing"

	"github.com/alecthomas/assert/v2"
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
