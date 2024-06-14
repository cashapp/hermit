package github_test

import (
	"flag"
	"io"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/github"
	"gopkg.in/dnaeon/go-vcr.v3/recorder"
)

var updateRecordings = flag.Bool("update", false, "update test recordings")

func TestProjectForURL(t *testing.T) {
	tests := []struct {
		give string
		want string
	}{
		{"https://github.com/cashapp/hermit", "cashapp/hermit"},
		{"https://github.com/cashapp/hermit/", "cashapp/hermit"},
		{"https://github.com/cashapp/hermit/releases", "cashapp/hermit"},
		{"https://github.com/cashapp", ""},
		{"https://exmaple.com/cashapp/hermit", ""},
	}

	for _, tt := range tests {
		t.Run(tt.give, func(t *testing.T) {
			client := github.New(nil, "")
			got := client.ProjectForURL(tt.give)
			assert.Equal(t, tt.want, got)
		})
	}
}

func newVCRClient(t *testing.T) *github.Client {
	t.Helper()

	casetteMode := recorder.ModeReplayOnly
	if *updateRecordings {
		casetteMode = recorder.ModeRecordOnly
	} else {
		t.Cleanup(func() {
			if t.Failed() {
				t.Logf("Run the following command to update the test recording:")
				t.Logf("  go test -run '^%s$' -update", t.Name())
			}
		})
	}

	recorder, err := recorder.NewWithOptions(&recorder.Options{
		CassetteName: filepath.Join("testdata", t.Name()),
		Mode:         casetteMode,
	})
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, recorder.Stop())
	})

	return github.New(recorder.GetDefaultClient(), "")
}

func TestRepo(t *testing.T) {
	client := newVCRClient(t)
	repo, err := client.Repo("cashapp/hermit")
	assert.NoError(t, err)

	assert.Equal(t, "https://cashapp.github.io/hermit", repo.Homepage)
}

func TestRelease(t *testing.T) {
	client := newVCRClient(t)
	release, err := client.Release("cashapp/hermit", "v0.39.2")
	assert.NoError(t, err)

	assert.Equal(t, "v0.39.2", release.TagName)
	assert.Equal(t, 6, len(release.Assets))

	// The cheapest asset to download is the install.sh script.
	var installSh *github.Asset
	for idx, asset := range release.Assets {
		if asset.Name == "install.sh" {
			installSh = &release.Assets[idx]
		}
	}
	assert.NotZero(t, installSh, "install.sh not found in assets")

	t.Run("ETag", func(t *testing.T) {
		etag, err := client.ETag(*installSh)
		assert.NoError(t, err)
		assert.NotZero(t, etag)
	})

	t.Run("Download", func(t *testing.T) {
		res, err := client.Download(*installSh)
		assert.NoError(t, err)
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		assert.NoError(t, err)

		assert.Contains(t, string(body), "hermit")
	})
}

func TestLatestRelease(t *testing.T) {
	client := newVCRClient(t)
	release, err := client.LatestRelease("cashapp/hermit")
	assert.NoError(t, err)
	assert.NotZero(t, release.TagName)
}

func TestReleases(t *testing.T) {
	client := newVCRClient(t)
	releases, err := client.Releases("cashapp/hermit-build", 5)
	assert.NoError(t, err)
	assert.Equal(t, 5, len(releases))
}
