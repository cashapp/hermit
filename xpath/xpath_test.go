package xpath

import (
	"strings"
	"testing"

	"aqwari.net/xml/xmltree"
	"github.com/stretchr/testify/require"
)

func TestXPath(t *testing.T) {
	root, err := xmltree.Parse([]byte(`
		<root>
			<el id="id" />
			<el a="one" b="two" />
			<el id="id" name="foo"/>
		</root>
	`))
	require.NoError(t, err)
	tests := []struct {
		name     string
		sel      string
		expected string
	}{
		{"MultipleByName", `/root/el`, `<el id="id" /><el a="one" b="two" /><el id="id" name="foo" />`},
		{"MultipleByWildcard", `/root/*`, `<el id="id" /><el a="one" b="two" /><el id="id" name="foo" />`},
		{"ByAttr", `/root/el[@id = "id"]`, `<el id="id" /><el id="id" name="foo" />`},
		{"ByMultipleAttrs", `/root/el[@id = "id" and @name="foo"]`, `<el id="id" name="foo" />`},
		{"ByAttrNameOnly", `/root/el[@id]`, `<el id="id" /><el id="id" name="foo" />`},
	}
	for _, test := range tests {
		path := MustParse(test.sel)
		t.Run(test.name, func(t *testing.T) {
			sel := path.Select(root)
			s := []string{}
			for _, el := range sel {
				s = append(s, el.String())
			}
			require.Equal(t, test.expected, strings.Join(s, ""))
		})
	}
}
