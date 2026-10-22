package platform

import (
	"cmp"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/cashapp/hermit/errors"
)

// GlibcVersion is a glibc release version.
//
// The zero value means glibc is absent or its version is unknown.
type GlibcVersion struct {
	Major int
	Minor int
}

// ParseGlibcVersion parses a glibc version of the form "<major>.<minor>", eg. "2.28".
func ParseGlibcVersion(s string) (GlibcVersion, error) {
	majorStr, minorStr, ok := strings.Cut(s, ".")
	major, majorErr := strconv.ParseUint(majorStr, 10, 31)
	minor, minorErr := strconv.ParseUint(minorStr, 10, 31)
	if !ok || majorErr != nil || minorErr != nil {
		return GlibcVersion{}, errors.Errorf("invalid glibc version %q, expected <major>.<minor>", s)
	}
	return GlibcVersion{Major: int(major), Minor: int(minor)}, nil
}

func (v GlibcVersion) IsZero() bool { return v == GlibcVersion{} }

// Compare returns -1, 0, or +1 if v is older than, the same as, or newer than other.
func (v GlibcVersion) Compare(other GlibcVersion) int {
	return cmp.Or(cmp.Compare(v.Major, other.Major), cmp.Compare(v.Minor, other.Minor))
}

func (v GlibcVersion) String() string {
	return strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor)
}

// hostGlibc is detected once per process because detection runs a subprocess.
var hostGlibc = sync.OnceValue(func() GlibcVersion {
	if runtime.GOOS != Linux {
		return GlibcVersion{}
	}
	out, err := exec.Command("getconf", "GNU_LIBC_VERSION").Output()
	if err != nil {
		return GlibcVersion{}
	}
	return parseGetconfGlibcVersion(string(out))
})

// parseGetconfGlibcVersion parses the output of "getconf GNU_LIBC_VERSION", eg. "glibc 2.35".
//
// Returns the zero version if the output is not recognised.
func parseGetconfGlibcVersion(out string) GlibcVersion {
	name, version, ok := strings.Cut(strings.TrimSpace(out), " ")
	if !ok || name != "glibc" {
		return GlibcVersion{}
	}
	// Development snapshots report a third component, eg. "2.39.9000".
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return GlibcVersion{}
	}
	v, err := ParseGlibcVersion(parts[0] + "." + parts[1])
	if err != nil {
		return GlibcVersion{}
	}
	return v
}

// Host returns the platform Hermit is running on.
//
// On Linux the glibc version is detected with getconf. Hosts without glibc,
// or where detection fails, have a zero glibc version.
func Host() Platform {
	return Platform{OS: runtime.GOOS, Arch: runtime.GOARCH, Glibc: hostGlibc()}
}
