package digest

import (
	"net/http"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/platform"
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

func TestWithGlibcVersions(t *testing.T) {
	glibc217 := platform.GlibcVersion{Major: 2, Minor: 17}
	glibc228 := platform.GlibcVersion{Major: 2, Minor: 28}
	got := withGlibcVersions(platform.Platforms{
		{OS: platform.Linux, Arch: platform.Amd64},
		{OS: platform.Darwin, Arch: platform.Arm64},
	}, []platform.GlibcVersion{glibc217, glibc228})
	assert.Equal(t, platform.Platforms{
		{OS: platform.Linux, Arch: platform.Amd64},
		{OS: platform.Darwin, Arch: platform.Arm64},
		{OS: platform.Linux, Arch: platform.Amd64, Glibc: glibc217},
		{OS: platform.Linux, Arch: platform.Amd64, Glibc: glibc228},
	}, got)
	assert.True(t, isCore(got[2]))
}
