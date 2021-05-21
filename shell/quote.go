package shell

import (
	"bytes"
	"strings"
)

// Quote returns a word quoted with single-quotes for consumption by shells.
//
// This is necessary because kballard/go-shellquote does not quote leading "=T", which in zsh
// is for some unfathomable reason equivalent to $(which t) and breaks :(
// http://zsh.sourceforge.net/Doc/Release/Expansion.html#g_t_0060_003d_0027-expansion
//
// Copied from MIT licensed:
// https://github.com/kballard/go-shellquote/blob/95032a82bc518f77982ea72343cc1ade730072f0/quote.go#L70
func Quote(word string) string {
	var buf bytes.Buffer
	// quote mode
	// Use single-quotes, but if we find a single-quote in the word, we need
	// to terminate the string, emit an escaped quote, and start the string up
	// again
	inQuote := false
	for len(word) > 0 {
		i := strings.IndexRune(word, '\'')
		if i == -1 {
			break
		}
		if i > 0 {
			if !inQuote {
				buf.WriteByte('\'')
				inQuote = true
			}
			buf.WriteString(word[0:i])
		}
		word = word[i+1:]
		if inQuote {
			buf.WriteByte('\'')
			inQuote = false
		}
		buf.WriteString("\\'")
	}
	if len(word) > 0 {
		if !inQuote {
			buf.WriteByte('\'')
		}
		buf.WriteString(word)
		buf.WriteByte('\'')
	}
	return buf.String()
}
