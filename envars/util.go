package envars

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cashapp/hermit/platform"
)

// Parse a KEY=VALUE list of environment variables into an [Envars] map.
func Parse(envars []string) Envars {
	env := make(Envars, len(envars))
	for _, envar := range envars {
		parts := strings.SplitN(envar, "=", 2)
		env[parts[0]] = parts[1]
	}
	return env
}

// ExpandNoEscape behaves the same as [Expand] but without replacing "$$" with "$".
func ExpandNoEscape(s string, mapping func(string) (string, bool)) string {
	last := ""
	for last != s {
		last = s
		s = os.Expand(s, func(s string) string {
			value, ok := mapping(s)
			if !ok {
				return s
			}
			return value
		})
	}
	return s
}

// Expand repeatedly expands s until the mapping function stops making
// substitutions. This is useful for variables that reference other variables.
// Instances of "$$" are escaped to "$".
func Expand(s string, mapping func(string) (string, bool)) string {
	return strings.ReplaceAll(ExpandNoEscape(s, mapping), "$$", "$")
}

// Mapping returns a function that expands hermit variables. The function can
// be passed to [Expand]. Instances of "$$" are left unexpanded.
func Mapping(env, home string, p platform.Platform) func(s string) (string, bool) {
	return func(key string) (string, bool) {
		switch key {
		case "HERMIT_ENV", "env":
			return env, true

		case "HERMIT_BIN":
			return filepath.Join(env, "bin"), true

		case "os":
			return p.OS, true

		case "arch":
			return p.Arch, true

		case "xarch":
			if xarch := platform.ArchToXArch(p.Arch); xarch != "" {
				return xarch, true
			}
			return p.Arch, true

		case "HOME":
			return home, true

		case "YYYY":
			return fmt.Sprintf("%04d", time.Now().Year()), true

		case "MM":
			return fmt.Sprintf("%02d", time.Now().Month()), true

		case "DD":
			return fmt.Sprintf("%02d", time.Now().Day()), true

		case "$":
			// Pass through "$$" unmodified. [os.Expand] does not provide the ability
			// to escape "$", so we need to handle it ourselves. Since [Expand] loops until
			// no more changes are made, we cannot transform "$$" to "$" or else the result
			// itself will be expanded. For example, "$$foo" -> "$foo" -> "value_of_foo".
			// See https://github.com/golang/go/issues/43482
			return "$$", true

		default:
			return "", false
		}
	}
}
