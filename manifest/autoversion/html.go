package autoversion

import (
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/antchfx/htmlquery"
	"github.com/pkg/errors"
	"golang.org/x/net/html"

	"github.com/cashapp/hermit/manifest"
)

// Auto-version by extracting version information from a HTML URL using XPath.
func htmlAutoVersion(client *http.Client, autoVersion *manifest.AutoVersionBlock) (version string, err error) {
	versionRe, err := regexp.Compile(autoVersion.VersionPattern)
	if err != nil {
		return "", errors.WithStack(err)
	}
	url := autoVersion.HTML.URL
	resp, err := client.Get(url) // nolint
	if err != nil {
		return "", errors.Wrapf(err, "could not retrieve auto-version information")
	}
	defer resp.Body.Close()
	node, err := html.Parse(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "%s: could not parse HTML", url)
	}
	matches, err := htmlquery.QueryAll(node, autoVersion.HTML.XPath)
	if err != nil {
		return "", errors.Wrap(err, autoVersion.HTML.XPath)
	}
	if len(matches) == 0 {
		return "", nil
	}
	// Parse and sort versions so we can get the latest.
	versions := make(manifest.Versions, 0, len(matches))
	for _, match := range matches {
		switch match.Type {
		case html.TextNode:
			groups := versionRe.FindStringSubmatch(match.Data)
			if groups == nil {
				return "", errors.Errorf("version must match the pattern %s but is %s", autoVersion.VersionPattern, match.Data)
			}
			versions = append(versions, manifest.ParseVersion(groups[1]))

		default:
			w := &strings.Builder{}
			_ = html.Render(w, match)
			return "", errors.Errorf("XPath query %q did not select a text node, selected: %s", autoVersion.HTML.XPath, w.String())
		}
	}
	sort.Sort(versions)
	return versions[len(versions)-1].String(), nil
}
