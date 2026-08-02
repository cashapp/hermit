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

func TestEnvName(t *testing.T) {
	tests := []struct {
		name     string
		root     string
		expected string
	}{
		{"Plain", "/tmp/hermit-env", "hermit-env"},
		{"Spaces", "/tmp/Application Support/hermit env", "hermit env"},
		{"SafePunctuation", "/tmp/a-b_c.d,e+f=g:h@i~j", "a-b_c.d,e+f=g:h@i~j"},
		{"Unicode", "/tmp/héllo-wörld-日本語", "héllo-wörld-日本語"},
		{"CommandSubstitution", "/tmp/$(id)", "__id_"},
		{"Backticks", "/tmp/`id`", "_id_"},
		{"ParameterExpansion", "/tmp/${HOME}", "__HOME_"},
		{"Quotes", `/tmp/a"b'c`, "a_b_c"},
		{"Backslash", `/tmp/a\b\`, "a_b_"},
		{"BashPromptEscape", `/tmp/\u\h\$`, "_u_h__"},
		{"ZshPromptEscape", "/tmp/%n%m%(e)", "_n_m__e_"},
		{"ZshHistoryExpansion", "/tmp/a!b", "a_b"},
		{"Control", "/tmp/a\nb\rc\td\x00e", "a_b_c_d_e"},
		{"Semicolons", "/tmp/a;b&c|d", "a_b_c_d"},
		{"InvalidUTF8", "/tmp/a\xff\xfeb", "a__b"},
		{"Root", "/", "_"},
		{"Empty", "", "."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, envName(test.root))
		})
	}
}
