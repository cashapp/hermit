package manifest

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	versionRe      = regexp.MustCompile(`^([^-+a-zA-Z]+)(?:[-]?([a-zA-Z][^+]*))?(?:\+(.+))?$`)
	versionPartsRe = regexp.MustCompile(`[._]`)
)

// Version of a package.
//
// This is very loosely parsed to support the myriad different ways of
// specifying versions, though it does attempt to support semver
// prerelease and metadata.
type Version struct {
	orig       string
	version    []string // The main .-separated parts of the version.
	prerelease []string // .-separated parts of the pre-release component.
	metadata   string   // Optional metadata after +
}

// Versions sortable.
type Versions []Version

func (v Versions) Len() int           { return len(v) }
func (v Versions) Less(i, j int) bool { return v[i].Compare(v[j]) < 0 }
func (v Versions) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }

// ParseVersion "parses" a package version.
func ParseVersion(version string) Version {
	var (
		components []string
		prerelease []string
		metadata   string
		parts      = versionRe.FindStringSubmatch(version)
	)
	if parts == nil {
		parts = []string{version, version, "", ""}
	}
	if parts[1] != "" {
		components = versionPartsRe.Split(parts[1], -1)
	}
	if parts[2] != "" {
		prerelease = versionPartsRe.Split(parts[2], -1)
	}
	metadata = parts[3]
	return Version{
		orig:       version,
		version:    components,
		prerelease: prerelease,
		metadata:   metadata,
	}
}

// Components returns the numeric version number components of the
func (v Version) Components() []string {
	return v.version
}

// Prerelease data in the version, if any.
//
// See https://semver.org/#spec-item-9
func (v Version) Prerelease() string {
	return strings.Join(v.prerelease, ".")
}

// PrereleaseComponents returns the . separated parts of the prerelease section.
func (v Version) PrereleaseComponents() []string {
	return v.prerelease
}

// Metadata in the version, if any.
//
// See https://semver.org/#spec-item-10
func (v Version) Metadata() string {
	return v.metadata
}

func (v Version) String() string {
	return v.orig
}

func (v Version) MarshalJSON() ([]byte, error) {
	return []byte(`"` + v.String() + `"`), nil
}

// Clean pre-release and metadata.
func (v Version) Clean() Version {
	v.metadata = ""
	v.prerelease = nil
	v.orig = v.String()
	return v
}

// Major version number components only (includes prerelease + metadata).
func (v Version) Major() Version {
	return v.addPrereleaseMetadata(v.version[0])
}

// MajorMinor version number components only (includes prerelease + metadata).
func (v Version) MajorMinor() Version {
	if len(v.version) == 1 {
		return v.addPrereleaseMetadata(v.version[0])
	}
	return v.addPrereleaseMetadata(strings.Join(v.version[:2], "."))
}

func (v Version) addPrereleaseMetadata(seed string) Version {
	if v.prerelease != nil {
		seed += "-" + strings.Join(v.prerelease, ".")
	}
	if v.metadata != "" {
		seed += "+" + v.metadata
	}
	return ParseVersion(seed)
}

func (v Version) GoString() string {
	return fmt.Sprintf("manifest.ParseVersion(%q)", v)
}

// Compare two versions.
//
// This attempts to follow semantic versioning, ie. prereleases are not considered and metadata is ignored.
func (v Version) Compare(rhs Version) int {
	if len(v.prerelease) != 0 && len(rhs.prerelease) == 0 {
		return -1
	} else if len(v.prerelease) == 0 && len(rhs.prerelease) != 0 {
		return +1
	}
	n := compareVersionParts(v.version, rhs.version)
	if n == 0 {
		return compareVersionParts(v.prerelease, rhs.prerelease)
	}
	return n
}

// IsSet returns true if the Version is set.
func (v Version) IsSet() bool { return v.orig != "" }

// Match returns true if this version is equal to or a more general version of "other".
//
// That is, if "other" is "1.2.3" and our version is "1.2" this will return true.
func (v Version) Match(other Version) bool {
	for i := range v.version {
		if i >= len(other.version) {
			return false
		}
		if v.version[i] != other.version[i] {
			return false
		}
	}
	for i := range v.prerelease {
		if i >= len(other.prerelease) {
			return false
		}
		if v.prerelease[i] != other.prerelease[i] {
			return false
		}
	}
	if v.metadata != "" && v.metadata != other.metadata {
		return false
	}
	return true
}

// Less return true if this version is smaller than the given version
func (v Version) Less(other Version) bool {
	return v.Compare(other) < 0
}

// References is a sortable list of Reference's.
type References []Reference

func (r References) Len() int           { return len(r) }
func (r References) Less(i, j int) bool { return r[i].Less(r[j]) }
func (r References) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }

// A Reference to a package, potentially only providing partial versions, etc.
type Reference struct {
	Name    string
	Version Version
	Channel string
}

// ParseReference parses a name+version for a package.
func ParseReference(pkg string) Reference {
	p := Reference{}
	parts := strings.SplitN(pkg, "@", 2)
	if len(parts) > 1 {
		p.Channel = parts[1]
	}
	pkg = parts[0]
	pver := parts[0]
	cursor := 0
	// Look for a hyphen followed by a number.
	for {
		i := strings.IndexAny(pver[cursor:], "-")
		// No version included.
		if i < 0 {
			p.Name = pkg
			return p
		}
		cursor += i + 1
		rn := rune(pver[cursor])
		if cursor < len(pver) && unicode.IsDigit(rn) {
			pkg = pkg[:cursor-1]
			pver = pver[cursor:]
			break
		}
	}
	p.Name = pkg
	p.Version = ParseVersion(pver)
	return p
}

func (r Reference) GoString() string {
	return fmt.Sprintf("manifest.ParseReference(%q)", r)
}

// IsSet returns true if the Reference isn't empty.
func (r Reference) IsSet() bool {
	return r.Name != ""
}

// IsFullyQualified returns true if the Reference is fully qualified, ie. has a version or channel.
func (r Reference) IsFullyQualified() bool {
	return r.Name != "" && (r.Version.IsSet() || r.Channel != "")
}

// IsChannel returns true if the reference refers to a channel package
func (r Reference) IsChannel() bool {
	return r.Channel != ""
}

func (r Reference) String() string {
	out := r.Name
	if r.Version.String() != "" {
		out += "-" + r.Version.String()
	}
	if r.Channel != "" {
		out += "@" + r.Channel
	}
	return out
}

// StringNoName returns the formatted version+channel portion of the reference.
func (r Reference) StringNoName() string {
	out := r.Version.String()
	if r.Channel != "" {
		out += "@" + r.Channel
	}
	return out
}

// Major package-major of the package reference.
func (r Reference) Major() Reference {
	if !r.Version.IsSet() {
		return r
	}
	return Reference{
		Name:    r.Name,
		Version: r.Version.Major(),
	}
}

// MajorMinor package-major.minor of the package reference.
func (r Reference) MajorMinor() Reference {
	if !r.Version.IsSet() {
		return r
	}
	return Reference{
		Name:    r.Name,
		Version: r.Version.MajorMinor(),
	}
}

// Less returns true if other is less than us.
func (r Reference) Less(other Reference) bool {
	if r.Name < other.Name {
		return true
	}
	// Channels always rank lower.
	if r.Channel != "" && other.Channel == "" {
		return true
	} else if r.Channel != "" && other.Channel != "" && r.Channel < other.Channel {
		return true
	}
	return r.Version.Less(other.Version)
}

// Compare reference to another, returning -1, 0, or +1.
func (r Reference) Compare(other Reference) int {
	if n := strings.Compare(r.Name, other.Name); n != 0 {
		return n
	}
	if r.Channel != "" && other.Channel == "" {
		return -1
	}
	if r.Channel != "" || other.Channel != "" {
		if n := strings.Compare(r.Channel, other.Channel); n != 0 {
			return n
		}
	}
	return r.Version.Compare(other.Version)
}

// Match returns true if the name and version components we have match those of other.
func (r Reference) Match(other Reference) bool {
	if r.Name != other.Name {
		return false
	}
	if (r.Channel != "" || other.Channel != "") && r.Channel != other.Channel {
		return false
	}
	return r.Version.Match(other.Version)
}

func compareVersionParts(lhs, rhs []string) int {
	for i, left := range lhs {
		if i >= len(rhs) {
			return 1
		}
		right := rhs[i]
		leftn, lerr := strconv.ParseInt(left, 10, 64)
		rightn, rerr := strconv.ParseInt(right, 10, 64)
		if lerr != nil || rerr != nil {
			if left < right {
				return -1
			} else if left > right {
				return +1
			}
		} else {
			if leftn < rightn {
				return -1
			} else if leftn > rightn {
				return +1
			}
		}
	}
	return len(lhs) - len(rhs)
}
