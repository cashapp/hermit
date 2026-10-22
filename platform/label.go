package platform

import (
	"regexp"
	"slices"
	"strings"

	"github.com/cashapp/hermit/errors"
)

// glibcLabelPrefix marks a label as a glibc version requirement rather than a regex.
// No OS or architecture starts with "glibc", and older Hermit versions skip these labels.
const glibcLabelPrefix = "glibc"

// Match returns true if the label matches the platform. A label is either glibc_<major>_<minor>,
// requiring that glibc version or newer, or a regex matched against the OS, arch, and <os>-<arch>.
func (p Platform) Match(label string) (bool, error) {
	required, isGlibc, err := parseGlibcLabel(label)
	if err != nil {
		return false, err
	}
	if isGlibc {
		return !p.Glibc.IsZero() && p.Glibc.Compare(required) >= 0, nil
	}
	re, err := regexp.Compile(label)
	if err != nil {
		return false, errors.Wrapf(err, "invalid platform label %q", label)
	}
	return re.MatchString(p.OS) || re.MatchString(p.Arch) || re.MatchString(p.OS+"-"+p.Arch), nil
}

// Variants returns p followed by a variant for each glibc version the labels require, oldest first.
// Only Linux platforms have glibc variants.
func (p Platform) Variants(labels []string) (Platforms, error) {
	var glibcVersions []GlibcVersion
	for _, label := range labels {
		required, isGlibc, err := parseGlibcLabel(label)
		if err != nil {
			return nil, err
		}
		if isGlibc && !slices.Contains(glibcVersions, required) {
			glibcVersions = append(glibcVersions, required)
		}
	}
	variants := Platforms{p}
	if p.OS != Linux {
		return variants, nil
	}
	slices.SortFunc(glibcVersions, GlibcVersion.Compare)
	for _, glibc := range glibcVersions {
		variant := p
		variant.Glibc = glibc
		variants = append(variants, variant)
	}
	return variants, nil
}

// parseGlibcLabel parses a label of the form "glibc_<major>_<minor>", eg. "glibc_2_28".
// isGlibc is false if the label is not a glibc label.
func parseGlibcLabel(label string) (required GlibcVersion, isGlibc bool, err error) {
	rest, ok := strings.CutPrefix(label, glibcLabelPrefix)
	if !ok {
		return GlibcVersion{}, false, nil
	}
	rest, hasSeparator := strings.CutPrefix(rest, "_")
	major, minor, hasMinor := strings.Cut(rest, "_")
	required, err = ParseGlibcVersion(major + "." + minor)
	if !hasSeparator || !hasMinor || err != nil {
		return GlibcVersion{}, true, errors.Errorf("invalid platform label %q, expected glibc_<major>_<minor>", label)
	}
	return required, true, nil
}
