package manifest

import (
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/alecthomas/hcl"

	"github.com/cashapp/hermit/envars"
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

// Help returns the manifest help.
func Help() string {
	ast, err := hcl.Schema(&Manifest{})
	if err != nil {
		panic(err)
	}
	w := &strings.Builder{}
	err = hcl.MarshalASTToWriter(ast, w)
	if err != nil {
		panic(err)
	}
	return w.String()
}

// A Layer contributes to the final merged manifest definition.
type Layer struct {
	Arch     string            `hcl:"arch,optional" help:"CPU architecture to match (amd64, 386, arm, etc.)."`
	Binaries []string          `hcl:"binaries,optional" help:"Relative glob from $root to individual terminal binaries."`
	Apps     []string          `hcl:"apps,optional" help:"Relative paths to Mac .app packages to install."`
	Rename   map[string]string `hcl:"rename,optional" help:"Rename files after unpacking to ${root}."`
	Requires []string          `hcl:"requires,optional" help:"Packages this one requires."`
	Provides []string          `hcl:"provides,optional" help:"This package provides the given virtual packages."`
	Dest     string            `hcl:"dest,optional" help:"Override archive extraction destination for package."`
	Files    map[string]string `hcl:"files,optional" help:"Files to load strings from to be used in the manifest."`
	Strip    int               `hcl:"strip,optional" help:"Number of path prefix elements to strip."`
	Root     string            `hcl:"root,optional" help:"Override root for package."`
	Test     *string           `hcl:"test,optional" help:"Command that will test the package is operational."`
	Env      envars.Envars     `hcl:"env,optional" help:"Environment variables to export."`
	Source   string            `hcl:"source,optional" help:"URL for source package."`
	Mirrors  []string          `hcl:"mirrors,optional" help:"Mirrors to use if the primary source is unavailable."`
	SHA256   string            `hcl:"sha256,optional" help:"SHA256 of source package for verification."`
	Darwin   []*Layer          `hcl:"darwin,block" help:"Darwin-specific configuration."`
	Linux    []*Layer          `hcl:"linux,block" help:"Linux-specific configuration."`
	Triggers []*Trigger        `hcl:"on,block" help:"Triggers to run on lifecycle events."`
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
	return out
}

func (c *Layer) match(arch string) bool {
	return c.Arch == "" || c.Arch == arch
}

// VersionBlock is a Layer block specifying an installable version of a package.
type VersionBlock struct {
	Version []string `hcl:"version,label" help:"Version(s) of package."`
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
