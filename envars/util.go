package envars

import (
	"strings"
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
