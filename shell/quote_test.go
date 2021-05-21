package shell

import (
	"testing"

	"github.com/kballard/go-shellquote"
	"github.com/stretchr/testify/require"
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
		require.Equal(t, test.quoted, Quote(test.original))
		original, err := shellquote.Split(test.quoted)
		require.NoError(t, err)
		require.Equal(t, []string{test.original}, original)
	}
}
