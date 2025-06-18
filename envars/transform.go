package envars

import (
	"os"
	"strings"
)

// Transform encapsulates low-level transformations on an existing environment.
type Transform struct {
	seed    Envars
	dest    Envars
	envRoot string
}

// Changed returns the set of changed Envars.
//
// If "undo" is true the returned Envars will include undo state.
func (t *Transform) Changed(undo bool) Envars {
	if undo {
		return t.dest.Clone()
	}
	out := make(Envars, len(t.dest))
	for k, v := range t.dest {
		if strings.HasPrefix(k, "_HERMIT_OLD_") || v == "" {
			continue
		}
		out[k] = v
	}
	return out
}

// Combined returns a copy of the full set of original Envars with Changed applied.
//
// Deleted keys will be removed.
func (t *Transform) Combined() Envars {
	out := t.seed.Clone()
	t.To(out)
	return out
}

// To applies the Transform to "env" in place.
func (t *Transform) To(env Envars) {
	for k, v := range t.dest {
		if v == "" {
			delete(env, k)
		} else {
			env[k] = v
		}
	}
}

// get a value.
func (t *Transform) get(key string) (string, bool) {
	// [os.Expand] does not allow you to escape "$", so we handle
	// it here. If a string contains e.g. "$${foo}", we will receive
	// "$" as an argument here. Simply return "$" to have that end
	// up expanding to "${foo}".
	if key == "$" {
		return "$", true
	}
	if v, ok := t.dest[key]; ok {
		return v, ok
	}
	v, ok := t.seed[key]
	return v, ok
}

// Set a key, expanding any ${X} references in the value.
func (t *Transform) set(key, value string) {
	t.dest[key] = t.expand(value)
}

// Unset a key.
func (t *Transform) unset(key string) {
	t.dest[key] = ""
}

func (t *Transform) expand(value string) string {
	return os.Expand(value, func(s string) string {
		v, _ := t.get(s)
		return v
	})
}
