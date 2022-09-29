package shell

import (
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/kballard/go-shellquote"
)

func TestShellQuote(t *testing.T) {
	tests := []struct {
		original string
		quoted   string
	}{
		{`=test`, `'=test'`},
		{`"hello world"`, `'"hello world"'`},
		{`'hello' 'world'`, `\''hello'\'' '\''world'\'`},
	}
	for _, test := range tests {
		assert.Equal(t, test.quoted, Quote(test.original))
		original, err := shellquote.Split(test.quoted)
		assert.NoError(t, err)
		assert.Equal(t, []string{test.original}, original)
	}
}
