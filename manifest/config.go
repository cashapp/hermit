package manifest

import (
	"reflect"
	"slices"
	"time"

	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/platform"
	"github.com/cashapp/hermit/redact"
)

//go:generate stringer -linecomment -type PackageState

// PackageState is the state a package is in.
type PackageState int

// Different states a package can be in.
const (
	PackageStateRemote     PackageState = iota // remote
	PackageStateDownloaded                     // downloaded
	PackageStateInstalled                      // installed
)

// A Layer contributes to the final merged manifest definition.
type Layer struct {
	Arch         string            `hcl:"arch,optional" help:"CPU architecture to match (amd64, 386, arm, etc.)."`
	Binaries     []string          `hcl:"binaries,optional" help:"Relative glob from $root to individual terminal binaries."`
	Apps         []string          `hcl:"apps,optional" help:"Relative paths to Mac .app packages to install."`
	Rename       map[string]string `hcl:"rename,optional" help:"Rename files after unpacking to ${root}."`
	Requires     []string          `hcl:"requires,optional" help:"Packages this one requires."`
	RuntimeDeps  []string          `hcl:"runtime-dependencies,optional" help:"Packages used internally by this package, but not installed to the target environment"`
	Provides     []string          `hcl:"provides,optional" help:"This package provides the given virtual packages."`
	Dest         string            `hcl:"dest,optional" help:"Override archive extraction destination for package."`
	Files        map[string]string `hcl:"files,optional" help:"Files to load strings from to be used in the manifest."`
	Strip        int               `hcl:"strip,optional" help:"Number of path prefix elements to strip."`
	Root         string            `hcl:"root,optional" help:"Override root for package."`
	Test         *string           `hcl:"test,optional" help:"Command that will test the package is operational."`
	Env          envars.Envars     `hcl:"env,optional" help:"Environment variables to export."`
	Vars         map[string]string `hcl:"vars,optional" help:"Set local variables used during manifest evaluation."`
	Source       redact.URL        `hcl:"source,optional" help:"URL for source package. Valid URLs are Git repositories (using .git[#<tag>] suffix), Local Files (using file:// prefix), and Remote Files (using http:// or https:// prefix)"`
	DontExtract  bool              `hcl:"dont-extract,optional" help:"Don't extract the package source, just copy it into the installation directory."`
	Mirrors      []redact.URL      `hcl:"mirrors,optional" help:"Mirrors to use if the primary source is unavailable."`
	SHA256       string            `hcl:"sha256,optional" help:"SHA256 of source package for verification. When in conflict with SHA256 in sha256sums, this value takes precedence."`
	SHA256Source redact.URL        `hcl:"sha256-source,optional" help:"URL for SHA256 checksum file for source package."`
	Darwin       []*Layer          `hcl:"darwin,block" help:"Darwin-specific configuration."`
	Linux        []*Layer          `hcl:"linux,block" help:"Linux-specific configuration."`
	Platform     []*PlatformBlock  `hcl:"platform,block" help:"Platform-specific configuration. <attr> is a set of regexes that must all match against one of CPU, OS, etc., or glibc_<major>_<minor>. When several blocks match, later blocks override earlier ones."`
	Triggers     []*Trigger        `hcl:"on,block" help:"Triggers to run on lifecycle events."`
	Mutable      bool              `hcl:"mutable,optional" help:"Package will not be made read-only."`
}

func (c Layer) layers(p platform.Platform) (out layers, err error) {
	out = append(out, &c)
	var selected []*Layer
	switch p.OS {
	case "darwin":
		selected = c.Darwin
	case "linux":
		selected = c.Linux
	}
	for _, layer := range selected {
		if layer.match(p.Arch) {
			out = append(out, layer)
		}
	}
	for _, block := range c.Platform {
		matched, err := block.match(p)
		if err != nil {
			return nil, err
		}
		if matched {
			out = append(out, &block.Layer)
		}
	}
	return out, nil
}

func (c *Layer) match(arch string) bool {
	return c.Arch == "" || c.Arch == arch
}

// AutoVersionBlock represents auto-version configuration.
type AutoVersionBlock struct {
	GitHubRelease string                `hcl:"github-release,optional" help:"GitHub <user>/<repo> to retrieve and update versions from the releases API."`
	HTML          *HTMLAutoVersionBlock `hcl:"html,block" help:"Extract version information from a HTML URL using XPath."`
	JSON          *JSONAutoVersionBlock `hcl:"json,block" help:"Extract version information from a JSON URL using jq."`
	GitTags       redact.URL            `hcl:"git-tags,optional" help:"Git remote URL to fetch git tags for version extraction."`

	VersionPattern        string `hcl:"version-pattern,optional" help:"Regex with one capture group to extract the version number from the origin." default:"v?(.*)"`
	IgnoreInvalidVersions bool   `hcl:"ignore-invalid-versions,optional" help:"Ignore tags that don't match the versin-pattern instead of failing. Does not apply to versions extracted using HTML URL"`
}

// HTMLAutoVersionBlock defines how version numbers can be extracted from HTML.
type HTMLAutoVersionBlock struct {
	URL   string `hcl:"url" help:"URL to retrieve HTML from."`
	XPath string `hcl:"xpath,optional" help:"XPath for selecting versions from HTML (see https://github.com/antchfx/htmlquery) - use version-pattern to extract substrings"`
	CSS   string `hcl:"css,optional" help:"CSS selector for selecting versions from HTML (see https://github.com/andybalholm/cascadia). Only one of xpath or css can be specified."`
}

// JSONAutoVersionBlock defines how version numbers can be extracted from a JSON API using jq.
type JSONAutoVersionBlock struct {
	URL string `hcl:"url" help:"URL to retrieve JSON from."`
	JQ  string `hcl:"jq" help:"jq expression to extract version strings from JSON (see https://github.com/itchyny/gojq)."`
}

// PlatformBlock matches a set of attributes describing a platform (eg. CPU, OS, etc.)
//
// The PlatformBlock replaces "linux" and "darwin".
type PlatformBlock struct {
	Attrs []string `hcl:"attr,label" help:"Regexes to match against platform attributes <arch>, <os>, and <os>-<arch>, or glibc_<major>_<minor> to require that glibc version or newer."`
	Layer
}

// match returns true if every label of the block matches the platform.
func (b *PlatformBlock) match(p platform.Platform) (bool, error) {
	for _, label := range b.Attrs {
		matched, err := p.Match(label)
		if err != nil || !matched {
			return false, err
		}
	}
	return true, nil
}

// validate returns an error for the first invalid label of the block.
func (b *PlatformBlock) validate() error {
	for _, label := range b.Attrs {
		// Match reports invalid labels regardless of the platform.
		if _, err := (platform.Platform{}).Match(label); err != nil {
			return err
		}
	}
	return nil
}

// VersionBlock is a Layer block specifying an installable version of a package.
type VersionBlock struct {
	Version     []string          `hcl:"version,label" help:"Version(s) of package."`
	AutoVersion *AutoVersionBlock `hcl:"auto-version,block" help:"Automatically update versions."`
	Layer
}

// ChannelBlock is a Layer block specifying an installable channel for a package.
type ChannelBlock struct {
	Name    string        `hcl:"name,label" help:"Name of the channel (eg. stable, alpha, etc.)."`
	Update  time.Duration `hcl:"update" help:"Update frequency for this channel."`
	Version string        `hcl:"version,optional" help:"Use the latest version matching this version glob as the source of this channel. Empty string matches all versions"`
	Layer
}

func (c *ChannelBlock) layersWithReferences(p platform.Platform, m *Manifest) (layers, error) {
	layer, err := c.layers(p)
	if err != nil {
		return nil, err
	}
	if c.Version != "" {
		v := c.Version
		g, err := ParseGlob(v)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		result, _ := m.HighestMatch(g)
		if result != nil {
			versionLayers, err := result.layers(p)
			if err != nil {
				return nil, err
			}
			return append(versionLayers, layer...), nil
		}

		return nil, errors.Errorf("@%s: no version found matching %s", c.Name, v)
	}

	return layer, nil
}

// Manifest for a package.
type Manifest struct {
	Layer
	Default     string            `hcl:"default,optional" help:"Default version or channel if not specified."`
	Description string            `hcl:"description" help:"Human readable description of the package."`
	Homepage    string            `hcl:"homepage,optional" help:"Home page."`
	Repository  string            `hcl:"repository,optional" help:"Source Repository."`
	SHA256Sums  map[string]string `hcl:"sha256sums,optional" help:"SHA256 checksums of source packages for verification."`
	Versions    []VersionBlock    `hcl:"version,block" help:"Definition of and configuration for a specific version."`
	Channels    []ChannelBlock    `hcl:"channel,block" help:"Definition of and configuration for an auto-update channel."`
}

// Merge layers for the selected package reference, either from versions or channels.
func (m *Manifest) layers(ref Reference, p platform.Platform) (layers, error) {
	for _, v := range m.Versions {
		if !slices.Contains(v.Version, ref.Version.String()) {
			continue
		}
		l, err := v.layers(p)
		if err != nil {
			return nil, err
		}
		return m.withManifestLayers(p, l)
	}
	for _, ch := range m.Channels {
		if ch.Name == ref.Channel {
			l, err := ch.layersWithReferences(p, m)
			if err != nil {
				return nil, err
			}
			return m.withManifestLayers(p, l)
		}
	}
	return nil, nil
}

// withManifestLayers prepends the manifest's top-level layers to l.
func (m *Manifest) withManifestLayers(p platform.Platform, l layers) (layers, error) {
	base, err := m.Layer.layers(p)
	if err != nil {
		return nil, err
	}
	return append(base, l...), nil
}

// unsupported returns the base platforms where no variant supports the given Reference.
func (m *Manifest) unsupported(ref Reference, base platform.Platforms) (platform.Platforms, error) {
	variants, err := m.PlatformVariants(base)
	if err != nil {
		return nil, err
	}
	supported := map[platform.Platform]bool{}
	for _, p := range variants {
		lrs, err := m.layers(ref, p)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(lrs, func(l *Layer) bool { return l.Source != "" }) {
			supported[p.Base()] = true
		}
	}
	var result platform.Platforms
	for _, p := range base {
		if !supported[p.Base()] {
			result = append(result, p)
		}
	}
	return result, nil
}

// platformBlocks returns the platform blocks that layers() can select.
func (m *Manifest) platformBlocks() []*PlatformBlock {
	blocks := slices.Clone(m.Platform)
	for _, v := range m.Versions {
		blocks = append(blocks, v.Platform...)
	}
	for _, ch := range m.Channels {
		blocks = append(blocks, ch.Platform...)
	}
	return blocks
}

// PlatformVariants returns each base platform followed by its variants that the manifest's platform blocks distinguish.
//
// Resolving every variant reaches every platform block, which a fixed list of base platforms cannot do.
func (m *Manifest) PlatformVariants(base platform.Platforms) (platform.Platforms, error) {
	var labels []string
	for _, block := range m.platformBlocks() {
		labels = append(labels, block.Attrs...)
	}
	var variants platform.Platforms
	for _, p := range base {
		pv, err := p.Variants(labels)
		if err != nil {
			return nil, err
		}
		variants = append(variants, pv...)
	}
	return variants, nil
}

// GetVersions returns all the versions defined in this manifest
func (m *Manifest) GetVersions() []Version {
	var result []Version
	for _, vs := range m.Versions {
		for _, v := range vs.Version {
			result = append(result, ParseVersion(v))
		}
	}
	return result
}

// GetChannels returns all the channels defined in this manifest.
func (m *Manifest) GetChannels() []string {
	result := make([]string, len(m.Channels))
	for i, c := range m.Channels {
		result[i] = c.Name
	}
	return result
}

// References defined in this manifest
func (m *Manifest) References(name string) References {
	versions := m.GetVersions()
	channels := m.GetChannels()

	refs := make(References, len(versions)+len(channels))
	for i, v := range versions {
		refs[i] = Reference{Name: name, Version: v}
	}
	for i, c := range channels {
		refs[i+len(versions)] = Reference{Name: name, Channel: c}
	}
	return refs
}

type layers []*Layer

// Return the last non-zero value for a field in the stack of layers.
func (ls layers) field(key string, seed interface{}) interface{} {
	out := seed
	for _, l := range ls {
		f := reflect.ValueOf(l).Elem().FieldByName(key)
		if !f.IsZero() {
			out = f.Interface()
		}
	}
	return out
}
