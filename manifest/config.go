package manifest

import (
	"reflect"
	"regexp"
	"time"

	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/platform"
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
	Arch        string            `hcl:"arch,optional" help:"CPU architecture to match (amd64, 386, arm, etc.)."`
	Binaries    []string          `hcl:"binaries,optional" help:"Relative glob from $root to individual terminal binaries."`
	Apps        []string          `hcl:"apps,optional" help:"Relative paths to Mac .app packages to install."`
	Rename      map[string]string `hcl:"rename,optional" help:"Rename files after unpacking to ${root}."`
	Requires    []string          `hcl:"requires,optional" help:"Packages this one requires."`
	RuntimeDeps []string          `hcl:"runtime-dependencies,optional" help:"Packages used internally by this package, but not installed to the target environment"`
	Provides    []string          `hcl:"provides,optional" help:"This package provides the given virtual packages."`
	Dest        string            `hcl:"dest,optional" help:"Override archive extraction destination for package."`
	Files       map[string]string `hcl:"files,optional" help:"Files to load strings from to be used in the manifest."`
	Strip       int               `hcl:"strip,optional" help:"Number of path prefix elements to strip."`
	Root        string            `hcl:"root,optional" help:"Override root for package."`
	Test        *string           `hcl:"test,optional" help:"Command that will test the package is operational."`
	Env         envars.Envars     `hcl:"env,optional" help:"Environment variables to export."`
	Vars        map[string]string `hcl:"vars,optional" help:"Set local variables used during manifest evaluation."`
	Source      string            `hcl:"source,optional" help:"URL for source package. Valid URLs are Git repositories (using .git[#<tag>] suffix), Local Files (using file:// prefix), and Remote Files (using http:// or https:// prefix)"`
	Mirrors     []string          `hcl:"mirrors,optional" help:"Mirrors to use if the primary source is unavailable."`
	SHA256      string            `hcl:"sha256,optional" help:"SHA256 of source package for verification."`
	Darwin      []*Layer          `hcl:"darwin,block" help:"Darwin-specific configuration."`
	Linux       []*Layer          `hcl:"linux,block" help:"Linux-specific configuration."`
	Platform    []*PlatformBlock  `hcl:"platform,block" help:"Platform-specific configuration. <attr> is a set regexes that must all match against one of CPU, OS, etc.."`
	Triggers    []*Trigger        `hcl:"on,block" help:"Triggers to run on lifecycle events."`
	Mutable     bool              `hcl:"mutable,optional" help:"Package will not be made read-only."`
}

func (c Layer) layers(os string, arch string) (out layers) {
	out = append(out, &c)
	var selected []*Layer
	switch os {
	case "darwin":
		selected = c.Darwin
	case "linux":
		selected = c.Linux
	}
	if len(selected) != 0 {
		for _, layer := range selected {
			if layer.match(arch) {
				out = append(out, layer)
			}
		}
	}
nextPlatform:
	for _, platform := range c.Platform {
		for _, attr := range platform.Attrs {
			re, err := regexp.Compile(attr)
			if err != nil {
				continue
			}
			if !re.MatchString(os) && !re.MatchString(arch) {
				continue nextPlatform
			}
		}
		out = append(out, &platform.Layer)
	}
	return out
}

func (c *Layer) match(arch string) bool {
	return c.Arch == "" || c.Arch == arch
}

// AutoVersionBlock represents auto-version configuration.
type AutoVersionBlock struct {
	GitHubRelease string                `hcl:"github-release,optional" help:"GitHub <user>/<repo> to retrieve and update versions from the releases API."`
	HTML          *HTMLAutoVersionBlock `hcl:"html,block" help:"Extract version information from a HTML URL using XPath."`

	VersionPattern string `hcl:"version-pattern,optional" help:"Regex with one capture group to extract the version number from the origin." default:"v?(.*)"`
}

// HTMLAutoVersionBlock defines how version numbers can be extracted from HTML.
type HTMLAutoVersionBlock struct {
	URL   string `hcl:"url" help:"URL to retrieve HTML from."`
	XPath string `hcl:"xpath" help:"XPath for extracting versions from HTML (see https://github.com/antchfx/htmlquery)"`
}

// PlatformBlock matches a set of attributes describing a platform (eg. CPU, OS, etc.)
//
// The PlatformBlock replaces "linux" and "darwin".
type PlatformBlock struct {
	Attrs []string `hcl:"attr,label" help:"Platform attributes to match."`
	Layer
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

func (c *ChannelBlock) layersWithReferences(os string, arch string, m *Manifest) (layers, error) {
	layer := c.layers(os, arch)
	if c.Version != "" {
		v := c.Version
		g, err := ParseGlob(v)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		result, _ := m.HighestMatch(g)
		if result != nil {
			return append(result.layers(os, arch), layer...), nil
		}

		return nil, errors.Errorf("@%s: no version found matching %s", c.Name, v)
	}

	return layer, nil
}

// Manifest for a package.
type Manifest struct {
	Layer
	Default     string         `hcl:"default,optional" help:"Default version or channel if not specified."`
	Description string         `hcl:"description" help:"Human readable description of the package."`
	Homepage    string         `hcl:"homepage,optional" help:"Home page."`
	Repository  string         `hcl:"repository,optional" help:"Source Repository."`
	Versions    []VersionBlock `hcl:"version,block" help:"Definition of and configuration for a specific version."`
	Channels    []ChannelBlock `hcl:"channel,block" help:"Definition of and configuration for an auto-update channel."`
}

// Merge layers for the selected package reference, either from versions or channels.
func (m *Manifest) layers(ref Reference, os string, arch string) (layers, error) {
	versionLayers := map[string]layers{}

	for _, v := range m.Versions {
		l := v.layers(os, arch)
		for _, version := range v.Version {
			versionLayers[version] = l
			if version == ref.Version.String() {
				return append(m.Layer.layers(os, arch), l...), nil
			}
		}
	}
	for _, ch := range m.Channels {
		if ch.Name == ref.Channel {
			l, err := ch.layersWithReferences(os, arch, m)
			if err != nil {
				return nil, err
			}
			return append(m.Layer.layers(os, arch), l...), nil
		}
	}
	return nil, nil
}

// unsupported returns the platforms not supported in the given Reference
func (m *Manifest) unsupported(ref Reference, platforms []platform.Platform) []platform.Platform {
	var result []platform.Platform
platformsNext:
	for _, p := range platforms {
		lrs, _ := m.layers(ref, p.OS, p.Arch)
		for _, l := range lrs {
			if l.Source != "" {
				continue platformsNext
			}
		}
		result = append(result, p)
	}
	return result
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
