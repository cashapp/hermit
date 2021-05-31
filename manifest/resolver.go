package manifest

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gobwas/glob"

	"github.com/pkg/errors"
	"github.com/qdm12/reprint"

	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
)

// ErrUnknownPackage is returned when a package cannot be resolved.
var ErrUnknownPackage = errors.New("unknown package")
var xarch = map[string]string{
	"amd64": "x86_64",
	"386":   "i386",
	"arm64": "aarch64",
}

// Config required for loading manifests.
type Config struct {
	// Path to environment root.
	Env string
	// State path where packages are installed.
	State string
	// Optional OS (will use runtime.GOOS if not provided).
	OS string
	// Optional Arch (will use runtime.GOARCH if not provided).
	Arch string
}

// Packages sortable by name + version.
//
// Prerelease versions will sort as the oldest versions.
type Packages []*Package

func (p Packages) Len() int { return len(p) }
func (p Packages) Less(i, j int) bool {
	n := strings.Compare(p[i].Reference.Name, p[j].Reference.Name)
	if n == 0 {
		return p[i].Reference.Less(p[j].Reference)
	}
	return n < 0
}
func (p Packages) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

// ResolvedFileRef contains information of a file that should be copied to the target package
// after unpacking
type ResolvedFileRef struct {
	FS       fs.FS
	FromPath string
	ToPAth   string
}

// Package resolved from a manifest.
type Package struct {
	Description    string
	Reference      Reference
	Arch           string
	Binaries       []string
	Apps           []string
	Requires       []string
	Provides       []string
	Rename         map[string]string `json:"-"`
	Env            envars.Ops
	Source         string
	Mirrors        []string
	Root           string
	SHA256         string
	Dest           string
	Test           string
	Strip          int
	Triggers       map[Event][]Action `json:"-"` // Triggers keyed by event.
	UpdateInterval time.Duration      // How often should we check for updates? 0, if never
	Files          []*ResolvedFileRef `json:"-"`
	FS             fs.FS              `json:"-"` // FS the Package was loaded from.
	Warnings       []string           `json:"-"`

	// Filled in by Env.
	Linked    bool `json:"-"` // Linked into environment.
	State     PackageState
	LastUsed  time.Time
	ETag      string
	UpdatedAt time.Time
}

func (p *Package) String() string {
	return p.Reference.String()
}

// Trigger triggers an event in this package. Noop if the event is not defined for the package
func (p *Package) Trigger(l ui.Logger, event Event) (messages []string, err error) {
	for _, action := range p.Triggers[event] {
		l.Debugf("%s", action)
		switch action := action.(type) {
		case *RunAction:
			if err := p.triggerRun(action); err != nil {
				return nil, errors.Wrapf(err, "%s: on %s", p, event)
			}

		case *CopyAction:
			if err := p.triggerCopy(action); err != nil {
				return nil, errors.Wrapf(err, "%s: on %s", p, event)
			}

		case *ChmodAction:
			if err := p.triggerChmod(action); err != nil {
				return nil, errors.Wrapf(err, "%s: on %s", p, event)
			}

		case *RenameAction:
			if err := p.triggerRename(action); err != nil {
				return nil, errors.Wrapf(err, "%s: on %s", p, event)
			}

		case *MessageAction:
			messages = append(messages, action.Text)

		default:
			panic("??")
		}
	}
	return messages, nil
}

// ResolveBinaries resolves binary globs from the filesystem.
func (p *Package) ResolveBinaries() ([]string, error) {
	// Expand binaries globs.
	binaries := make([]string, 0, len(p.Binaries))
	for _, bin := range p.Binaries {
		bin = path.Join(p.Root, bin)
		bins, err := filepath.Glob(bin)
		if err != nil {
			return nil, errors.Wrapf(err, "%s: failed to find binaries %q", p, bin)
		}
		if len(bins) == 0 {
			return nil, errors.Errorf("%s: failed to find binaries %q", p, bin)
		}
		binaries = append(binaries, bins...)
	}
	return binaries, nil
}

// LogWarnings logs possible warnings found in the package manifest
func (p *Package) LogWarnings(l *ui.UI) {
	task := l.Task(p.Reference.String())
	for _, warning := range p.Warnings {
		task.Warnf(warning)
	}
}

// ApplyEnvironment applies the env ops defined in the Package to the given environment.
func (p *Package) ApplyEnvironment(envRoot string, env envars.Envars) {
	env.Apply(envRoot, p.Env).To(env)
}

// DeprecationWarningf adds a new deprecation warning to the Package's warnings.
func (p *Package) DeprecationWarningf(format string, args ...interface{}) {
	p.Warnings = append(p.Warnings, fmt.Sprintf("DEPRECATED: "+format, args...))
}

// Resolver of packages.
type Resolver struct {
	config  Config
	sources *sources.Sources
	loader  *Loader
}

// New constructs a new package loader.
func New(sources *sources.Sources, config Config) (*Resolver, error) {
	if config.OS == "" {
		config.OS = runtime.GOOS
	}
	if config.Arch == "" {
		config.Arch = runtime.GOARCH
	}
	return &Resolver{
		config:  config,
		sources: sources,
		loader:  NewLoader(sources),
	}, nil
}

// LoadAll manifests.
func (r *Resolver) LoadAll() error {
	_, err := r.loader.All()
	return err
}

// Errors returns all errors encountered _so far_ by the Loader.
func (r *Resolver) Errors() ManifestErrors {
	return r.loader.Errors()
}

// Sync the sources of this resolver.
func (r *Resolver) Sync(l *ui.UI, force bool) error {
	if err := r.sources.Sync(l, force); err != nil {
		return errors.WithStack(err)
	}
	r.loader = NewLoader(r.sources)
	return nil
}

// Search for packages using the given regular expression.
func (r *Resolver) Search(l ui.Logger, pattern string) (Packages, error) {
	re, err := regexp.Compile("(?i)^(" + pattern + ")$")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var pkgs Packages
	manifests, err := r.loader.All()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, manifest := range manifests {
		if !re.MatchString(manifest.Name) {
			continue
		}
		for _, version := range manifest.Versions {
			ref := Reference{manifest.Name, ParseVersion(version.Version), ""}
			// If the reference doesn't resolve, discard it.
			pkg, err := newPackage(manifest, r.config, ExactSelector(ref))
			if err != nil {
				l.Warnf("invalid manifest reference %s in %s.hcl: %s", ref, manifest.Name, err)
				continue
			}
			pkgs = append(pkgs, pkg)
		}
		for _, channel := range manifest.Channels {
			name := filepath.Base(strings.TrimSuffix(manifest.Path, ".hcl"))
			ref := Reference{name, Version{}, channel.Name}
			// If the reference doesn't resolve, discard it.
			pkg, err := newPackage(manifest, r.config, ExactSelector(ref))
			if err != nil {
				l.Warnf("invalid manifest reference %s in %s.hcl: %s", ref, name, err)
				continue
			}
			pkgs = append(pkgs, pkg)
		}
	}
	sort.Sort(pkgs)
	return pkgs, nil
}

// ResolveVirtual references to concrete packages.
func (r *Resolver) ResolveVirtual(name string) (pkgs []*Package, err error) {
	manifests, err := r.loader.All()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var providers []*AnnotatedManifest
	for _, manifest := range manifests {
		for _, provides := range manifest.Provides {
			if provides == name {
				providers = append(providers, manifest)
			}
		}
	}
	if len(providers) == 0 {
		return nil, errors.Wrapf(ErrUnknownPackage, "unable to resolve virtual package %q", name)
	}
	for _, manifest := range providers {
		pkg, err := newPackage(manifest, r.config, NameSelector(name))
		if err != nil {
			return nil, err
		}
		pkg.Reference = ParseReference(manifest.Name)
		pkgs = append(pkgs, pkg)
	}
	return pkgs, nil
}

// Resolve a package reference.
//
// Returns the highest version matching the given reference
func (r *Resolver) Resolve(l *ui.UI, selector Selector) (pkg *Package, err error) {
	manifest, err := r.loader.Get(selector.Name())
	if err != nil {
		err := r.Sync(l, true)
		if err != nil {
			return nil, errors.Wrap(err, err.Error())
		}
		// Try again.
		manifest, err = r.loader.Get(selector.Name())
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return newPackage(manifest, r.config, selector)
}

func matchVersion(manifest *AnnotatedManifest, selector Selector) (collected References, selected Reference) {
	for _, v := range manifest.Versions {
		candidate := Reference{Name: selector.Name(), Version: ParseVersion(v.Version)}
		collected = append(collected, candidate)
		if selector.Matches(candidate) && (!selected.IsSet() || selected.Less(candidate)) {
			selected = candidate
		}
	}
	return
}

func matchChannel(manifest *AnnotatedManifest, selector Selector) (collected References, foundUpdateInterval time.Duration, selected Reference) {
	for _, ch := range manifest.Channels {
		candidate := Reference{Name: selector.Name(), Channel: ch.Name}
		collected = append(collected, candidate)
		if selector.Matches(candidate) {
			selected = candidate
			foundUpdateInterval = ch.Update
		}
	}
	return
}

func newPackage(manifest *AnnotatedManifest, config Config, selector Selector) (*Package, error) {
	// If a version was not specified and the manifest defines a default, use it.
	if !selector.IsFullyQualified() && manifest.Default != "" {
		if strings.HasPrefix(manifest.Default, "@") {
			selector = ExactSelector(Reference{Name: manifest.Name, Channel: manifest.Default[1:]})
		} else {
			m, err := GlobSelector(manifest.Name + "-" + manifest.Default)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			selector = m
		}
	}

	// Clone the entire manifest, as we mutate stuff.
	manifest = reprint.This(manifest).(*AnnotatedManifest)
	// Resolve version in manifest from ref.
	var foundUpdateInterval time.Duration
	// Search versions first.
	allRefs, found := matchVersion(manifest, selector)
	// Then channels if no match.
	if !found.IsSet() {
		var channelRefs References
		channelRefs, foundUpdateInterval, found = matchChannel(manifest, selector)
		allRefs = append(allRefs, channelRefs...)
	}
	// Finally just pick the most recent version.
	if !found.IsSet() && !selector.IsFullyQualified() {
		sort.Sort(allRefs)
		found = allRefs[len(allRefs)-1]
	}
	if !found.IsSet() {
		knownVersions := make([]string, 0, len(allRefs))
		for _, ref := range allRefs {
			knownVersions = append(knownVersions, ref.StringNoName())
		}
		sort.Strings(knownVersions)
		return nil, errors.Wrapf(ErrUnknownPackage, "%s: no version %s in known versions %s", manifest.Path, selector, strings.Join(knownVersions, ", "))
	}

	root := filepath.Join(config.State, "pkg", found.String())
	p := &Package{
		Description:    manifest.Description,
		Reference:      found,
		Rename:         map[string]string{},
		Root:           "${dest}",
		Dest:           root,
		Triggers:       map[Event][]Action{},
		UpdateInterval: foundUpdateInterval,
		Files:          []*ResolvedFileRef{},
		FS:             manifest.FS,
	}

	files := map[string]string{}

	// Merge all the layers.
	layers, err := manifest.layers(found, config.OS, config.Arch)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if found.IsChannel() {
		channel := manifest.ChannelByName(found.Channel)
		if channel != nil && channel.Version != "" {
			g, err := ParseGlob(channel.Version)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			highest := manifest.HighestMatch(g).Version
			found.Version = ParseVersion(highest)
		}
	}

	layerEnvars := make([]envars.Envars, 0, len(layers))
	for _, layer := range layers {
		if len(layer.Env) > 0 {
			layerEnvars = append(layerEnvars, layer.Env)
		}
		if layer.Arch != "" {
			p.Arch = layer.Arch
		}
		if layer.SHA256 != "" {
			p.SHA256 = layer.SHA256
		}
		if layer.Test != nil {
			p.Test = *layer.Test
		}
		if layer.Source != "" {
			p.Source = layer.Source
		}
		if len(layer.Mirrors) > 0 {
			p.Mirrors = layer.Mirrors
		}
		if layer.Root != "" {
			p.Root = layer.Root
		}
		if layer.Dest != "" {
			p.Dest = layer.Dest
		}
		if len(layer.Apps) != 0 {
			p.Apps = append(p.Apps, layer.Apps...)
		}
		if len(layer.Binaries) != 0 {
			p.Binaries = append(p.Binaries, layer.Binaries...)
		}
		if len(layer.Requires) != 0 {
			p.Requires = append(p.Requires, layer.Requires...)
		}
		if len(layer.Provides) != 0 {
			p.Provides = append(p.Provides, layer.Provides...)
		}
		for k, v := range layer.Rename {
			p.Rename[k] = v
		}
		if len(layer.Triggers) > 0 {
			for _, trigger := range layer.Triggers {
				p.Triggers[trigger.Event] = append(p.Triggers[trigger.Event], trigger.Ordered()...)
			}
		}
		for k, v := range layer.Files {
			files[k] = v
		}
	}
	// Validate.
	if len(p.Binaries) == 0 && len(p.Apps) == 0 {
		return nil, errors.Errorf("%s: %s: no binaries or apps provided", manifest.Path, found)
	}
	if p.Source == "" {
		return nil, errors.Errorf("%s: %s: no source provided", manifest.Path, found)
	}

	// Expand variables.
	//
	// If "ignoreMissing" is false, any referenced variables that are unknown will result in an error.
	//
	// TODO: Factor this out (there's a lot of captured state though).
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	mapping := func(ignoreMissing bool) func(s string) string {
		return func(key string) string {
			switch key {
			case "version":
				return found.Version.String()

			case "dest":
				return layers.field("Dest", p.Dest).(string)

			case "root":
				return layers.field("Root", p.Root).(string)

			case "HERMIT_ENV", "env":
				return config.Env

			case "HERMIT_BIN":
				return filepath.Join(config.Env, "bin")

			case "os":
				return config.OS

			case "arch":
				return config.Arch

			case "xarch":
				subst, ok := xarch[config.Arch]
				if ok {
					return subst
				}
				return config.Arch

			case "HOME":
				return home

			case "YYYY":
				return fmt.Sprintf("%04d", time.Now().Year())

			case "MM":
				return fmt.Sprintf("%02d", time.Now().Month())

			case "DD":
				return fmt.Sprintf("%02d", time.Now().Day())

			default:
				if ignoreMissing {
					return "${" + key + "}"
				}
				err = errors.Errorf("unknown variable $%s", key)
				return ""
			}
		}
	}

	// Expand envars in "s". If "ignoreMissing is true then unknown variable references will be
	// passed through unaltered.
	expand := func(s string, ignoreMissing bool) string {
		last := ""
		for strings.Contains(s, "${") && last != s {
			last = s
			s = os.Expand(s, mapping(ignoreMissing))
			if ignoreMissing {
				err = nil
			}
		}
		return s
	}

	for _, env := range layerEnvars {
		// Expand manifest variables but keep other variable references.
		for k, v := range env {
			env[k] = expand(v, true)
		}
		ops := envars.Infer(env.System())
		// Sort each layer of ops.
		sort.Slice(ops, func(i, j int) bool { return ops[i].Envar() < ops[j].Envar() })
		p.Env = append(p.Env, ops...)
	}
	p.Strip = layers.field("Strip", 0).(int)
	p.Dest = expand(p.Dest, false)
	p.Root = expand(p.Root, false)
	p.Test = expand(p.Test, false)
	for i, bin := range p.Binaries {
		p.Binaries[i] = expand(bin, false)
	}
	for i, requires := range p.Requires {
		p.Requires[i] = expand(requires, false)
	}
	for i, provides := range p.Provides {
		p.Provides[i] = expand(provides, false)
	}
	if len(p.Rename) > 0 {
		p.DeprecationWarningf(`rename = {"X": "Y"} must be replaced by on unpack { rename { from="${root}/X" to="${root}/Y" } }`)
	}
	for k, v := range p.Rename {
		delete(p.Rename, k)
		p.Rename[expand(k, true)] = expand(v, true)
	}
	p.Source = expand(p.Source, false)
	for i, mirror := range p.Mirrors {
		p.Mirrors[i] = expand(mirror, false)
	}
	for _, actions := range p.Triggers {
		for _, action := range actions {
			switch action := action.(type) {
			case *RunAction:
				for i, env := range action.Env {
					action.Env[i] = expand(env, false)
				}
				for i, arg := range action.Args {
					action.Args[i] = expand(arg, false)
				}
				action.Command = expand(action.Command, false)
				action.Dir = expand(action.Dir, false)

			case *CopyAction:
				action.From = expand(action.From, false)
				action.To = expand(action.To, false)

			case *ChmodAction:
				action.File = expand(action.File, false)

			case *RenameAction:
				action.From = expand(action.From, false)
				action.To = expand(action.To, false)

			case *MessageAction:
				action.Text = expand(action.Text, false)

			default:
				panic("??")
			}
		}
	}
	// This error is set by the mapping() function if ignoreMissing=false and a variable is missing.
	if err != nil {
		return nil, errors.WithStack(err)
	}

	for k, v := range files {
		files[k] = expand(v, false)
	}
	err = resolveFiles(manifest, p, files)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return p, err
}

func resolveFiles(manifest *AnnotatedManifest, pkg *Package, files map[string]string) error {
	if len(files) == 0 {
		return nil
	}

	for k, v := range files {
		f, err := manifest.FS.Open(k)
		if err != nil {
			return errors.WithStack(err)
		}
		err = f.Close()
		if err != nil {
			return errors.WithStack(err)
		}
		pkg.Files = append(pkg.Files, &ResolvedFileRef{
			FromPath: k,
			FS:       manifest.FS,
			ToPAth:   v,
		})
	}
	return nil
}

// Validate that there are no semantic errors in the manifest
func (m *Manifest) validate() []error {
	var (
		result   []error
		versions = m.Versions
	)

	for _, channel := range m.Channels {
		if channel.Version != "" {
			g, err := ParseGlob(channel.Version)
			if err != nil {
				result = append(result, errors.Errorf("@%s: invalid glob: %s", channel.Name, err))
			}
			found := false
			for _, v := range versions {
				if g.Match(ParseVersion(v.Version).String()) {
					found = true
					break
				}
			}
			if !found {
				result = append(result, errors.Errorf("@%s: no version found matching %s", channel.Name, channel.Version))
			}
		}
	}

	return result
}

// HighestMatch returns the VersionBlock with highest version number matching the given Glob
func (m *Manifest) HighestMatch(to glob.Glob) *VersionBlock {
	versions := m.Versions
	var result *VersionBlock
	var highest *Version
	for _, v := range versions {
		version := v
		parsed := ParseVersion(v.Version)
		if to.Match(v.Version) && (highest == nil || highest.Less(parsed)) {
			highest = &parsed
			result = &version
		}
	}
	return result
}

// ChannelByName returns the channel with the given name, or nil if not found
func (m *Manifest) ChannelByName(name string) *ChannelBlock {
	for _, c := range m.Channels {
		if c.Name == name {
			return &c
		}
	}
	return nil
}
