package envars

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/platform"
)

// validEnvKey matches names that are safe to interpolate directly into POSIX
// shell and fish scripts. The pattern is the standard POSIX identifier syntax
// used for environment variable names: an initial letter or underscore
// followed by any number of letters, digits, or underscores.
var validEnvKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateKey returns an error if key is not a valid POSIX environment
// variable name. Hermit emits keys verbatim into shell scripts, so anything
// outside this pattern is rejected to prevent shell command injection.
func ValidateKey(key string) error {
	if !validEnvKey.MatchString(key) {
		return errors.Errorf("invalid environment variable name %q (must match %s)", key, validEnvKey)
	}
	return nil
}

// Validate returns an error if any key in e is not a valid POSIX environment
// variable name. See [ValidateKey].
func (e Envars) Validate() error {
	for key := range e {
		if err := ValidateKey(key); err != nil {
			return err
		}
	}
	return nil
}

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
