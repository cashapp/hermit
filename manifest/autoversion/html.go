package autoversion

import (
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/andybalholm/cascadia"
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

	var matcher htmlMatcher
	switch {
	case autoVersion.HTML.XPath != "":
		matcher, err = compileXPathMatcher(autoVersion.HTML.XPath)
		if err != nil {
			return "", err
		}

	case autoVersion.HTML.CSS != "":
		matcher, err = compileCSSMatcher(autoVersion.HTML.CSS)
		if err != nil {
			return "", err
		}

	default:
		return "", errors.Errorf("must specify either xpath or css for auto-version html")
	}

	candidates, err := matcher.FindAll(node)
	if err != nil {
		return "", err
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

// htmlMatcher searches for strings inside an HTML document.
type htmlMatcher interface {
	FindAll(n *html.Node) ([]string, error)
}

// htmlXPathMatcher traverses HTML documents using a given XPath expression,
// and returns the text content of the selected nodes.
//
// The xpath expression must match an attribute, text, or element node,
// or produce a string value.
type htmlXPathMatcher struct {
	raw  string
	expr *xpath.Expr
}

// compileXPathMatcher compiles an XPath expression into a matcher.
func compileXPathMatcher(raw string) (*htmlXPathMatcher, error) {
	expr, err := xpath.Compile(raw)
	if err != nil {
		return nil, errors.Wrapf(err, "could not compile XPath expression %q", raw)
	}
	return &htmlXPathMatcher{raw: raw, expr: expr}, nil
}

func (m *htmlXPathMatcher) FindAll(node *html.Node) ([]string, error) {
	var candidates []string
	switch matches := m.expr.Evaluate(htmlquery.CreateXPathNavigator(node)).(type) {
	case *xpath.NodeIterator:
		for matches.MoveNext() {
			match := matches.Current()
			switch match.NodeType() {
			case xpath.AttributeNode, xpath.TextNode, xpath.ElementNode:
				candidates = append(candidates, match.Value())

			default:
				return nil, errors.Errorf("XPath query %q did not select a text or attribute node, selected node of type %d", m.raw, match.NodeType())
			}
		}

	case string:
		candidates = append(candidates, matches)

	default:
		return nil, errors.Errorf("XPath query %q did not select a text value, selected node of type %T", m.raw, matches)
	}

	return candidates, nil
}

// htmlCSSMatcher traverses HTML documents using a given CSS selector,
// and returns the text content of the selected nodes.
//
// The CSS selector must match a text or element node.
type htmlCSSMatcher struct {
	raw string
	sel cascadia.Selector
}

// compileCSSMatcher compiles a CSS selector into a matcher.
func compileCSSMatcher(raw string) (*htmlCSSMatcher, error) {
	sel, err := cascadia.Compile(raw)
	if err != nil {
		return nil, errors.Wrapf(err, "could not compile CSS selector %q", raw)
	}
	return &htmlCSSMatcher{raw: raw, sel: sel}, nil
}

func (m *htmlCSSMatcher) FindAll(node *html.Node) ([]string, error) {
	var candidates []string
	for _, match := range m.sel.MatchAll(node) {
		switch match.Type {
		case html.TextNode, html.ElementNode:
			candidates = append(candidates, strings.TrimSpace(match.FirstChild.Data))

		default:
			return nil, errors.Errorf("CSS selector %q did not select a text or element node, selected node of type %d", m.raw, match.Type)
		}
	}
	return candidates, nil
}
