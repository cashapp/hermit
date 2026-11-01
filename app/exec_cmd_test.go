package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"

	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/vfs"
)

func TestUpdateHermitChannel(t *testing.T) {
	tests := []struct {
		name         string
		lastCheckAge time.Duration // Zero means Hermit has never been checked.
		force        bool
		wantRequests int
		wantETag     string
	}{
		{name: "FirstUseSkipsCheck", wantRequests: 0, wantETag: ""},
		{name: "RecentCheckSkipsCheck", lastCheckAge: time.Hour, wantRequests: 0, wantETag: "old"},
		{name: "StaleCheckUpgrades", lastCheckAge: 25 * time.Hour, wantRequests: 2, wantETag: "new"},
		{name: "ForceUpgrades", lastCheckAge: time.Hour, force: true, wantRequests: 2, wantETag: "new"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("ETag", "new")
				if r.Method == http.MethodHead {
					return
				}
				fr, err := os.Open("../archive/testdata/archive.tar.gz")
				assert.NoError(t, err)
				defer fr.Close()
				_, err = io.Copy(w, fr)
				assert.NoError(t, err)
			}))
			defer server.Close()

			root := t.TempDir()
			t.Cleanup(func() {
				// Extracted packages are read-only.
				_ = filepath.Walk(root, func(path string, _ os.FileInfo, _ error) error {
					return os.Chmod(path, 0700) //nolint:gosec
				})
			})
			sta := openStateWithHermitManifest(t, root, server)

			// The last check time is the modification time of the package's metadata.
			metadata := filepath.Join(root, "metadata", "hermit@stable.etag")
			if test.lastCheckAge != 0 {
				assert.NoError(t, os.WriteFile(metadata, []byte("old"), 0600))
				lastCheck := time.Now().Add(-test.lastCheckAge)
				assert.NoError(t, os.Chtimes(metadata, lastCheck, lastCheck))
			}
			before, _ := os.Stat(metadata)

			l, _ := ui.NewForTesting()
			err := updateHermitChannel(l, sta, manifest.ParseReference("hermit@stable"), test.force)
			assert.NoError(t, err)

			assert.Equal(t, test.wantRequests, requests)
			etag, err := os.ReadFile(metadata)
			assert.NoError(t, err)
			assert.Equal(t, test.wantETag, string(etag))
			if test.lastCheckAge != 0 && test.wantRequests == 0 {
				after, err := os.Stat(metadata)
				assert.NoError(t, err)
				assert.Equal(t, before.ModTime(), after.ModTime(), "skipping a check must not restart the update interval")
			}
		})
	}
}

func openStateWithHermitManifest(t *testing.T, root string, server *httptest.Server) *state.State {
	t.Helper()
	client := server.Client()
	c, err := cache.Open(root, nil, client, client)
	assert.NoError(t, err)
	sta, err := state.Open(root, state.Config{
		Builtin: sources.NewBuiltInSource(vfs.InMemoryFS(map[string]string{
			"hermit.hcl": `
description = "Hermit"
binaries = ["darwin_exe"]

channel "stable" {
  source = "` + server.URL + `/hermit.tar.gz"
  update = "24h"
}
`,
		})),
	}, c)
	assert.NoError(t, err)
	return sta
}
