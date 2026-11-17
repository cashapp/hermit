package app

import (
	"bufio"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/gobwas/glob"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

// toolVersionsFile is the name of the file used by asdf and mise to pin tool versions.
const toolVersionsFile = ".tool-versions"

// toolVersion is a single entry in a .tool-versions file.
//
// An entry has one tool and one or more versions, in order of preference.
type toolVersion struct {
	Line     int
	Tool     string
	Versions []string
}

// asdf plugin names that differ from the corresponding Hermit package name.
var toolVersionsPackageNames = map[string]string{
	"nodejs":      "node",
	"golang":      "go",
	"python":      "python3",
	"dotnet-core": "dotnet",
	"aws-cli":     "awscli",
}

// parseToolVersions parses an asdf/mise style .tool-versions file.
//
// Each non-empty line is "<tool> <version> [<version>...]". Anything after a
// "#" is a comment.
func parseToolVersions(r io.Reader) ([]toolVersion, error) {
	var out []toolVersion
	scanner := bufio.NewScanner(r)
	line := 0
	for scanner.Scan() {
		line++
		text := scanner.Text()
		if i := strings.IndexByte(text, '#'); i >= 0 {
			text = text[:i]
		}
		fields := strings.Fields(text)
		if len(fields) == 0 {
			continue
		}
		if len(fields) == 1 {
			return nil, errors.Errorf("%s:%d: %q has no version", toolVersionsFile, line, fields[0])
		}
		out = append(out, toolVersion{Line: line, Tool: fields[0], Versions: fields[1:]})
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	return out, nil
}

// toolVersionSelectors returns the Hermit selectors to try for a single
// .tool-versions version, most specific first.
//
// An exact version is tried first, then the newest release with that version
// as a prefix (so "20.11" can match "20.11.1", as mise does). An error is
// returned if the version can not be expressed as a Hermit package.
func toolVersionSelectors(tool, version string) ([]manifest.GlobSelector, error) {
	name := tool
	if mapped, ok := toolVersionsPackageNames[tool]; ok {
		name = mapped
	}
	switch {
	case version == "system":
		return nil, errors.New("uses the system installation")
	case strings.HasPrefix(version, "ref:"), strings.HasPrefix(version, "path:"),
		strings.HasPrefix(version, "prefix:"), strings.HasPrefix(version, "sub-"):
		return nil, errors.Errorf("version %q is not supported", version)
	case version == "latest":
		sel, err := manifest.ParseGlobSelector(name)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return []manifest.GlobSelector{sel}, nil
	case strings.HasPrefix(version, "latest:"):
		return prefixSelectors(name, strings.TrimPrefix(version, "latest:"), false)
	}
	if tool == "java" {
		var err error
		name, version, err = javaPackage(version)
		if err != nil {
			return nil, err
		}
	}
	return prefixSelectors(name, version, true)
}

// javaPackage maps an asdf/mise Java version such as "temurin-17.0.2+8" or
// "corretto-21.0.5.11.1" to a Hermit package name and version.
func javaPackage(version string) (name string, pkgVersion string, err error) {
	vendor, rest, ok := strings.Cut(version, "-")
	if !ok || startsWithDigit(vendor) {
		// mise allows a bare version, which defaults to OpenJDK.
		return "openjdk", version, nil
	}
	switch vendor {
	case "temurin", "adoptopenjdk", "openjdk":
		// Hermit's openjdk package uses "_" where Adoptium uses "+".
		return "openjdk", strings.ReplaceAll(rest, "+", "_"), nil
	case "corretto":
		return "corretto", rest, nil
	default:
		return "", "", errors.Errorf("Java distribution %q has no Hermit equivalent", vendor)
	}
}

func prefixSelectors(name, version string, exact bool) ([]manifest.GlobSelector, error) {
	// Some tools are pinned with a leading "v" (eg. "v1.2.3").
	if len(version) > 1 && version[0] == 'v' && startsWithDigit(version[1:]) {
		version = version[1:]
	}
	if !startsWithDigit(version) {
		return nil, errors.Errorf("version %q is not supported", version)
	}
	quoted := glob.QuoteMeta(version)
	patterns := []string{}
	if exact {
		patterns = append(patterns, quoted)
	}
	patterns = append(patterns, quoted+".*")
	out := make([]manifest.GlobSelector, 0, len(patterns))
	for _, pattern := range patterns {
		sel, err := manifest.ParseGlobSelector(name + "-" + pattern)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		out = append(out, sel)
	}
	return out, nil
}

// exactSelector returns a selector that matches only ref.
func exactSelector(ref manifest.Reference) (manifest.GlobSelector, error) {
	if ref.IsChannel() {
		return manifest.ParseGlobSelector(ref.Name + "@" + ref.Channel)
	}
	return manifest.ParseGlobSelector(ref.Name + "-" + glob.QuoteMeta(ref.Version.String()))
}

func startsWithDigit(s string) bool {
	return s != "" && unicode.IsDigit(rune(s[0]))
}

// resolveToolVersion returns the first package that resolves for an entry.
func resolveToolVersion(l *ui.UI, env *hermit.Env, tv toolVersion) (*manifest.Package, error) {
	var reasons []string
	for _, version := range tv.Versions {
		selectors, err := toolVersionSelectors(tv.Tool, version)
		if err != nil {
			reasons = append(reasons, version+": "+err.Error())
			continue
		}
		for _, selector := range selectors {
			pkg, err := env.Resolve(l, selector, false)
			if err == nil {
				return pkg, nil
			}
			l.Debugf("%s:%d: %s did not resolve: %s", toolVersionsFile, tv.Line, selector, err)
		}
		reasons = append(reasons, version+": no matching Hermit package")
	}
	return nil, errors.New(strings.Join(reasons, "; "))
}

// importToolVersions installs the tools pinned in a .tool-versions file into
// env. Entries that can not be mapped to a Hermit package are reported as
// warnings and skipped, so that a partially compatible file still produces a
// usable environment.
func importToolVersions(l *ui.UI, env *hermit.Env, sta *state.State, path string) error {
	f, err := os.Open(path) // #nosec G304
	if err != nil {
		return errors.WithStack(err)
	}
	defer f.Close()
	entries, err := parseToolVersions(f)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	l.Infof("Importing %d tool(s) from %s", len(entries), path)
	// Sync sources once up front rather than on every failed lookup.
	if err := env.Update(l, true); err != nil {
		return errors.WithStack(err)
	}
	var selectors []manifest.GlobSelector
	seen := map[string]string{}
	for _, entry := range entries {
		pkg, err := resolveToolVersion(l, env, entry)
		if err != nil {
			l.Warnf("%s:%d: skipping %s (%s)", toolVersionsFile, entry.Line, entry.Tool, err)
			continue
		}
		if prev, ok := seen[pkg.Reference.Name]; ok {
			l.Warnf("%s:%d: skipping %s, %s is already selected", toolVersionsFile, entry.Line, pkg.Reference, prev)
			continue
		}
		seen[pkg.Reference.Name] = pkg.Reference.String()
		if len(entry.Versions) > 1 {
			l.Infof("%s:%d: Hermit installs one version per package, using %s", toolVersionsFile, entry.Line, pkg.Reference)
		}
		selector, err := exactSelector(pkg.Reference)
		if err != nil {
			return errors.WithStack(err)
		}
		selectors = append(selectors, selector)
	}
	if len(selectors) == 0 {
		return nil
	}
	install := &installCmd{Packages: selectors}
	return errors.Wrap(install.Run(l, env, sta), "install packages from "+toolVersionsFile)
}
