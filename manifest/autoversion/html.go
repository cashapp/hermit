package autoversion

import (
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/antchfx/htmlquery"
	"github.com/antchfx/xpath"
	"golang.org/x/net/html"

	"github.com/cashapp/hermit/errors"
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
	expr, err := xpath.Compile(autoVersion.HTML.XPath)
	if err != nil {
		return "", errors.Wrapf(err, "could not compile XPath expression %q", autoVersion.HTML.XPath)
	}

	// Collect potential candidates here.
	var candidates []string
	switch matches := expr.Evaluate(htmlquery.CreateXPathNavigator(node)).(type) {
	case *xpath.NodeIterator:
		for matches.MoveNext() {
			match := matches.Current()
			switch match.NodeType() {
			case xpath.AttributeNode, xpath.TextNode, xpath.ElementNode:
				candidates = append(candidates, match.Value())

			default:
				return "", errors.Errorf("XPath query %q did not select a text or attribute node, selected node of type %d", autoVersion.HTML.XPath, match.NodeType())
			}
		}

	case string:
		candidates = append(candidates, matches)

	default:
		return "", errors.Errorf("XPath query %q did not select a text value, selected node of type %T", autoVersion.HTML.XPath, matches)
	}

	// Parse and sort versions so we can get the latest.
	versions := make(manifest.Versions, 0, len(candidates))
	for _, value := range candidates {
		value = strings.TrimSpace(value)
		groups := versionRe.FindStringSubmatch(value)
		if groups == nil {
			return "", errors.Errorf("version must match the pattern %s but is %s", autoVersion.VersionPattern, value)
		}
		versions = append(versions, manifest.ParseVersion(groups[1]))
	}

	sort.Sort(versions)
	return versions[len(versions)-1].String(), nil
}
