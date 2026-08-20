// Package redact provides types for sensitive values that display in redacted
// form by default, and are only revealed at the point of deliberate use.
package redact

import (
	"fmt"
	"regexp"
	"strings"
)

var urlCredentialsRe = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/?#\s]+@`)

// Value is a possibly-sensitive value that displays in redacted form.
type Value interface {
	fmt.Stringer
	Reveal() string
}

// Plain is a non-sensitive value that displays as-is. It exists so that
// non-sensitive values can be mixed with sensitive ones, eg. in command arguments.
type Plain string

func (p Plain) String() string { return string(p) }
func (p Plain) Reveal() string { return string(p) }

// Secret is an opaque sensitive value, such as an access token, that displays
// as "[redacted]".
type Secret string

func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return "[redacted]"
}
func (s Secret) GoString() string { return s.String() }
func (s Secret) Reveal() string   { return string(s) }

// URL is a URL that may embed credentials in its userinfo section. It displays
// with the credentials removed, but serialises in full for config round-tripping.
type URL string

func (u URL) String() string                { return urlCredentialsRe.ReplaceAllString(string(u), "$1") }
func (u URL) GoString() string              { return u.String() }
func (u URL) Reveal() string                { return string(u) }
func (u URL) MarshalText() ([]byte, error)  { return []byte(u), nil }
func (u *URL) UnmarshalText(b []byte) error { *u = URL(b); return nil }

// Args wraps non-sensitive command arguments as a list of Values.
func Args(args ...string) []Value {
	out := make([]Value, len(args))
	for i, arg := range args {
		out[i] = Plain(arg)
	}
	return out
}

// Reveal returns the raw form of each value.
func Reveal(values []Value) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = value.Reveal()
	}
	return out
}

// Scrubber returns a Replacer substituting each sensitive value's raw form with
// its redacted form, for untyped text such as subprocess output. Nil if none.
func Scrubber(values []Value) *strings.Replacer {
	var pairs []string
	for _, value := range values {
		if value.Reveal() != value.String() {
			pairs = append(pairs, value.Reveal(), value.String())
		}
	}
	if pairs == nil {
		return nil
	}
	return strings.NewReplacer(pairs...)
}
