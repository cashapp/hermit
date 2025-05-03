package envars

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cashapp/hermit/platform"
)

// Parse a KEY=VALUE list of environment variables into an Envars map.
func Parse(envars []string) Envars {
	env := make(Envars, len(envars))
	for _, envar := range envars {
		parts := strings.SplitN(envar, "=", 2)
		env[parts[0]] = parts[1]
	}
	return env
}

// Expand repeatedly expands s until the mapping function stops making
// substitutions. This is useful for variables that reference other variables.
func Expand(s string, mapping func(string) string) string {
	last := ""
	for last != s {
		last = s
		s = os.Expand(s, mapping)
	}
	return s
}

// Mapping returns a function that expands hermit variables. The function can
// be passed to Expand.
func Mapping(env, home string, p platform.Platform) func(s string) string {
	return func(key string) string {
		switch key {
		case "HERMIT_ENV", "env":
			return env

		case "HERMIT_BIN":
			return filepath.Join(env, "bin")

		case "os":
			return p.OS

		case "arch":
			return p.Arch

		case "xarch":
			if xarch := platform.ArchToXArch(p.Arch); xarch != "" {
				return xarch
			}
			return p.Arch

		case "HOME":
			return home

		case "YYYY":
			return fmt.Sprintf("%04d", time.Now().Year())

		case "MM":
			return fmt.Sprintf("%02d", time.Now().Month())

		case "DD":
			return fmt.Sprintf("%02d", time.Now().Day())
		default:
			return ""
		}
	}
}
