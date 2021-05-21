// Package xpath is an XPath parser supporting the minimal subset of XPath
// used by Hermit. It exists so that only a single selector need be provided
// by package triggers, specifying the element to insert and the ability
// to find the parent under which to insert if the element does not exist.
//
// nolint: govet
package xpath

import (
	"fmt"
	"strings"

	"aqwari.net/xml/xmltree"
	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/stateful"
	"github.com/pkg/errors"
)

var (
	lex = lexer.Must(stateful.NewSimple([]stateful.Rule{
		{"Ident", `\w+`, nil},
		{"String", `"[^"]*"|'[^']*'`, nil},
		{"Whitespace", `\s+`, nil},
		{"Punct", `\[|\]|[@=/*]`, nil},
	}))
	parser = participle.MustBuild(&xpath{}, participle.Lexer(lex),
		participle.Unquote(), participle.Elide("Whitespace"))
)

// Parse a subset of XPath used in Hermit.
func Parse(sel string) (Path, error) {
	path := &xpath{}
	err := parser.ParseString(sel, path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return path.Component, nil
}

// MustParse parses a subset of XPath or panics.
func MustParse(sel string) Path {
	path, err := Parse(sel)
	if err != nil {
		panic(err)
	}
	return path
}

// An Attr on an element in the path.
type Attr struct {
	Key   string  `"@" @Ident`
	Value *string `("=" @String)?`
}

func (a *Attr) String() string {
	if a.Value == nil {
		return "@" + a.Key
	}
	return fmt.Sprintf("@%s=%q", a.Key, *a.Value)
}

// A Component is an element in a path selector.
type Component struct {
	Element string  `@(Ident | "*")`
	Attrs   []*Attr `("[" @@ ("and" @@)* "]")?`
}

func (c *Component) String() string {
	if len(c.Attrs) == 0 {
		return c.Element
	}
	attrs := []string{}
	for _, attr := range c.Attrs {
		attrs = append(attrs, attr.String())
	}
	return c.Element + "[" + strings.Join(attrs, " and ") + "]"
}

// Path components.
type Path []*Component

func (p Path) String() string {
	components := make([]string, len(p))
	for i, c := range p {
		components[i] = c.String()
	}
	return "/" + strings.Join(components, "/")
}

// Parent returns the Path that will select the parent element.
func (p Path) Parent() Path {
	if len(p) == 0 {
		return nil
	}
	return p[:len(p)-1]
}

// Select matches elements from a tree.
func (p Path) Select(root *xmltree.Element) (selected []*xmltree.Element) {
	if len(p) == 0 || root.Name.Local != p[0].Element && p[0].Element != "*" {
		return
	}
	for _, attr := range p[0].Attrs {
		xattr := root.Attr("", attr.Key)
		if xattr == "" || attr.Value != nil && *attr.Value != xattr {
			return
		}
	}
	next := p[1:]
	if len(next) == 0 {
		selected = append(selected, root)
		return
	}
	for i := range root.Children {
		if span := next.Select(&root.Children[i]); span != nil {
			selected = append(selected, span...)
		}
	}
	return
}

type xpath struct {
	Component []*Component `("/" @@)*`
}
