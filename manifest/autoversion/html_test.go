package autoversion

import (
	"net/http"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/cashapp/hermit/manifest"
)

func TestHTMLNoVersionsFound(t *testing.T) {
	tests := []struct {
		name      string
		htmlBlock *manifest.HTMLAutoVersionBlock
	}{
		{
			name: "XPath",
			htmlBlock: &manifest.HTMLAutoVersionBlock{
				URL:   "http://example.com",
				XPath: "/html/body/div",
			},
		},
		{
			name: "CSS",
			htmlBlock: &manifest.HTMLAutoVersionBlock{
				URL: "http://example.com",
				CSS: "body > div",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := htmlAutoVersion(&http.Client{
				Transport: testHTTPClient{
					path: "testdata/no_versions.html",
				},
			}, &manifest.AutoVersionBlock{
				HTML: tt.htmlBlock,
			})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "no versions matched")
		})
	}
}
