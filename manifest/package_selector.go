package manifest

import (
	"strings"
	"unicode"

	"github.com/gobwas/glob"
	"github.com/pkg/errors"
)

// Selector is a selector that matches package References and can be used to select a specific version of a package
type Selector interface {
	// Name of the package without version or channel qualifiers
	Name() string
	// String representation of this selector
	String() string
	// Matches checks if the selector matches this Reference
	Matches(ref Reference) bool
	// IsFullyQualified returns true if the selector specifies a version or a channel
	IsFullyQualified() bool
}

// selector where the source string is used as the UI string as well
type sourced struct {
	source string
}

func (m sourced) String() string {
	return m.source
}

// selector that is always fully qualified
type qualified struct{}

func (m qualified) IsFullyQualified() bool {
	return true
}

// GlobSelector selects matching packages using a glob.
//
// Can be used directly with Kong.
type GlobSelector struct {
	sourced
	name    string
	channel string
	version glob.Glob
}

func (m *GlobSelector) UnmarshalText(input []byte) error {
	gs, err := ParseGlobSelector(string(input))
	if err != nil {
		return errors.WithStack(err)
	}
	*m = gs
	return nil
}

func (m GlobSelector) IsFullyQualified() bool { // nolint
	return m.name != "" && (m.version != nil || m.channel != "")
}

func (m GlobSelector) Matches(ref Reference) bool { // nolint
	if ref.Name != m.name {
		return false
	}
	if m.channel != "" && ref.Channel != m.channel {
		return false
	}
	if m.version != nil && !m.version.Match(ref.Version.String()) {
		return false
	}
	return true
}

func (m GlobSelector) Name() string { // nolint
	return m.name
}

// ParseGlobSelector parses the given search string into a Glob based selector
func ParseGlobSelector(from string) (GlobSelector, error) {
	name, v, c := splitNameAndQualifier(from)
	var g glob.Glob
	if v != "" {
		compiled, err := glob.Compile(v)
		if err != nil {
			return GlobSelector{}, errors.WithStack(err)
		}
		g = compiled
	}

	return GlobSelector{sourced{from}, name, c, g}, nil
}

// MustParseGlobSelector or die.
func MustParseGlobSelector(from string) GlobSelector {
	sel, err := ParseGlobSelector(from)
	if err != nil {
		panic(err)
	}
	return sel
}

func splitNameAndQualifier(from string) (name string, version string, channel string) {
	for cursor := 0; cursor < len(from); cursor++ {
		rn := rune(from[cursor])
		if cursor > 0 && rune(from[cursor-1]) == '-' && (unicode.IsDigit(rn) || strings.ContainsRune("*[]{}", rn)) {
			return from[:cursor-1], from[cursor:], ""
		}
		if rn == '@' {
			return from[:cursor], "", from[cursor+1:]
		}
	}
	return from, "", ""
}

type nameSelector struct {
	sourced
	name string
}

func (m nameSelector) IsFullyQualified() bool {
	return false
}

func (m nameSelector) Matches(ref Reference) bool {
	return ref.Name == m.name
}

func (m nameSelector) Name() string {
	return m.name
}

// NameSelector returns a selector that matches all package versions of the given name
func NameSelector(name string) Selector {
	return nameSelector{
		sourced: sourced{name},
		name:    name,
	}
}

type exactSelector struct {
	qualified
	sourced
	ref Reference
}

func (m exactSelector) Matches(ref Reference) bool {
	return ref.String() == m.ref.String()
}

func (m exactSelector) Name() string {
	return m.ref.Name
}

// ExactSelector returns a selector that matches packages matching exactly the given reference
func ExactSelector(ref Reference) Selector {
	return exactSelector{
		sourced: sourced{ref.String()},
		ref:     ref,
	}
}

type prefixSelector struct {
	qualified
	sourced
	prefix Reference
}

func (m prefixSelector) Matches(ref Reference) bool {
	return m.prefix.Match(ref)
}

func (m prefixSelector) Name() string {
	return m.prefix.Name
}

// PrefixSelector returns a selector that matches packages with this reference as a prefix
func PrefixSelector(ref Reference) Selector {
	return prefixSelector{
		sourced: sourced{ref.String()},
		prefix:  ref,
	}
}

// ParseGlob parses a version glob into a Glob selector
func ParseGlob(from string) (glob.Glob, error) {
	return glob.Compile(from)
}
