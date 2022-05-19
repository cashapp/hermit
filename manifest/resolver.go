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

	"github.com/alecthomas/participle"
	"github.com/gobwas/glob"
	"github.com/qdm12/reprint"

	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/internal/system"
	"github.com/cashapp/hermit/platform"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
)

// ErrUnknownPackage is returned when a package cannot be resolved.
var ErrUnknownPackage = errors.New("unknown package")

// ErrNoBinaries is returned when a resolved package does not contain binaries or apps
var ErrNoBinaries = errors.New("no binaries or apps provided")

// ErrNoSource is returned when a resolved package does not contain source
var ErrNoSource = errors.New("no source provided")

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
	Description          string
	Homepage             string
	Reference            Reference
	Arch                 string
	Binaries             []string
	Apps                 []string
	Requires             []string
	RuntimeDeps          []Reference
	Provides             []string
	Env                  envars.Ops
	Source               string
	Mirrors              []string
	Root                 string
	SHA256               string
	Mutable              bool
	Dest                 string
	Test                 string
	Strip                int
	Triggers             map[Event][]Action  `json:"-"` // Triggers keyed by event.
	UpdateInterval       time.Duration       // How often should we check for updates? 0, if never
	Files                []*ResolvedFileRef  `json:"-"`
	FS                   fs.FS               `json:"-"` // FS the Package was loaded from.
	Warnings             []string            `json:"-"`
	UnsupportedPlatforms []platform.Platform // Unsupported core platforms

	// Filled in by Env.
	Linked    bool `json:"-"` // Linked into environment.
	State     PackageState
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
		if msg, ok := action.(*MessageAction); ok {
			messages = append(messages, msg.Text)
		} else if err := action.Apply(p); err != nil {
			return nil, errors.WithStack(err)
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

// Unsupported package in this environment.
func (p *Package) Unsupported() bool {
	return p.Source == ""
}

// EnsureSupported returns an error if the package is not supported on this platform
func (p *Package) EnsureSupported() error {
	if p.Unsupported() {
		return errors.Errorf("package %s is not supported on this architecture", p.Reference)
	}
	return nil
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
//
// Will be synced at most every SyncFrequency unless "force" is true.
//
// A Sources set can only be synchronised once. Following calls will not have any effect.
func (r *Resolver) Sync(l *ui.UI, force bool) error {
	if err := r.sources.Sync(l, force); err != nil {
		return errors.WithStack(err)
	}
	r.loader = NewLoader(r.sources)
	return nil
}

// Search for packages using the given regular expression.
func (r *Resolver) Search(l ui.Logger, pattern string) (Packages, error) {
	re, err := regexp.Compile("(?i)" + pattern + "")
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
			for _, vstr := range version.Version {
				ref := Reference{Name: manifest.Name, Version: ParseVersion(vstr)}
				// If the reference doesn't resolve, discard it.
				pkg, err := newPackage(manifest, r.config, ExactSelector(ref))
				if errors.Is(err, ErrNoSource) || errors.Is(err, ErrNoBinaries) || err == nil {
					pkgs = append(pkgs, pkg)
				} else {
					l.Warnf("invalid manifest reference %s in %s.hcl: %s", ref, manifest.Name, err)
					continue
				}
			}
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
	manifest, err := r.loader.Load(l, selector.Name())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return newPackage(manifest, r.config, selector)
}

func matchVersion(manifest *AnnotatedManifest, selector Selector) (collected References, selected Reference) {
	for _, v := range manifest.Versions {
		for _, vstr := range v.Version {
			candidate := Reference{Name: selector.Name(), Version: ParseVersion(vstr)}
			collected = append(collected, candidate)
			if selector.Matches(candidate) && (!selected.IsSet() || selected.Less(candidate)) {
				selected = candidate
			}
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
			m, err := ParseGlobSelector(manifest.Name + "-" + manifest.Default)
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
	if len(allRefs) == 0 {
		return nil, errors.Errorf("could not find any versions matching %s", selector)
	}
	// Finally just pick the most recent version.
	if !found.IsSet() && !selector.IsFullyQualified() {
		sort.Sort(allRefs)
		found = allRefs[len(allRefs)-1]
	}
	if !found.IsSet() {
		var knownVersions []string
		var knownChannels []string
		for _, ref := range allRefs {
			if ref.IsChannel() {
				knownChannels = append(knownChannels, ref.String())
			} else {
				knownVersions = append(knownVersions, ref.String())
			}
		}
		sort.Strings(knownVersions)
		sort.Strings(knownChannels)
		if strings.Contains(selector.String(), "@") {
			tryVersion := strings.ReplaceAll(selector.String(), "@", "-")
			for _, ver := range knownVersions {
				if ver == tryVersion {
					return nil, errors.Wrapf(ErrUnknownPackage, "%s: no channel %s found, did you mean version %s?",
						manifest.Path, selector, tryVersion)
				}
			}
			return nil, errors.Wrapf(ErrUnknownPackage, "%s: no channel %s found in channels (%s) or versions (%s)",
				manifest.Path, selector, strings.Join(knownChannels, ", "), strings.Join(knownVersions, ", "))
		}
		return nil, errors.Wrapf(ErrUnknownPackage, "%s: no version %s found in versions (%s) or channels (%s)",
			manifest.Path, selector, strings.Join(knownVersions, ", "), strings.Join(knownChannels, ", "))
	}

	root := filepath.Join(config.State, "pkg", found.String())
	p := &Package{
		Description:          manifest.Description,
		Homepage:             manifest.Homepage,
		Reference:            found,
		Root:                 "${dest}",
		Dest:                 root,
		Triggers:             map[Event][]Action{},
		UpdateInterval:       foundUpdateInterval,
		Files:                []*ResolvedFileRef{},
		FS:                   manifest.FS,
		UnsupportedPlatforms: manifest.unsupported(found, platform.Core),
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
			_, version := manifest.HighestMatch(g)
			if version == nil {
				return nil, errors.Errorf("no matching version found for channel %s", found)
			}
			found.Version = *version
		}
	}

	vars := map[string]string{}
	layerEnvars := make([]envars.Envars, 0, len(layers))
	for _, layer := range layers {
		if len(layer.Env) > 0 {
			layerEnvars = append(layerEnvars, layer.Env)
		}
		for k, v := range layer.Vars {
			vars[k] = v
		}
		if layer.Arch != "" {
			p.Arch = layer.Arch
		}
		if layer.SHA256 != "" {
			p.SHA256 = layer.SHA256
		}
		if layer.Mutable {
			p.Mutable = layer.Mutable
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
		if len(layer.Triggers) > 0 {
			for _, trigger := range layer.Triggers {
				p.Triggers[trigger.Event] = append(p.Triggers[trigger.Event], trigger.Ordered()...)
			}
		}
		if len(layer.RuntimeDeps) > 0 {
			for _, dep := range layer.RuntimeDeps {
				ref := ParseReference(dep)
				p.RuntimeDeps = append(p.RuntimeDeps, ref)
			}
		}
		for k, v := range layer.Files {
			files[k] = v
		}
	}
	// Verify.
	if len(p.Binaries) == 0 && len(p.Apps) == 0 {
		return p, errors.Wrapf(ErrNoBinaries, "%s: %s", manifest.Path, found)
	}
	if p.Source == "" {
		return p, errors.Wrapf(ErrNoSource, "%s: %s", manifest.Path, found)
	}

	// Expand variables.
	//
	// If "ignoreMissing" is false, any referenced variables that are unknown will result in an error.
	//
	// TODO: Factor this out (there's a lot of captured state though).
	home, err := system.UserHomeDir()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	mapping := func(ignoreMissing bool) func(s string) string {
		return func(key string) string {
			switch key {
			case "name":
				return found.Name

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
				if xarch := platform.ArchToXArch(config.Arch); xarch != "" {
					return xarch
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
				value, ok := vars[key]
				if ok {
					return value
				}
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
				if err := mustAbs(action, action.Command); err != nil {
					return nil, err
				}
				action.Dir = expand(action.Dir, false)
				if err := mustAbs(action, action.Dir); err != nil {
					return nil, err
				}

			case *CopyAction:
				action.From = expand(action.From, false)
				action.To = expand(action.To, false)
				if err := mustAbs(action, action.To); err != nil {
					return nil, err
				}

			case *ChmodAction:
				action.File = expand(action.File, false)
				if err := mustAbs(action, action.File); err != nil {
					return nil, err
				}

			case *RenameAction:
				action.From = expand(action.From, false)
				if err := mustAbs(action, action.From); err != nil {
					return nil, err
				}
				action.To = expand(action.To, false)
				if err := mustAbs(action, action.To); err != nil {
					return nil, err
				}

			case *DeleteAction:
				for i := range action.Files {
					action.Files[i] = expand(action.Files[i], false)
					if err := mustAbs(action, action.Files[i]); err != nil {
						return nil, err
					}
				}

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

// HighestMatch returns the VersionBlock with highest version number matching the given Glob
func (m *Manifest) HighestMatch(to glob.Glob) (result *VersionBlock, highest *Version) {
	versions := m.Versions
	for _, v := range versions {
		block := v
		for _, vstr := range v.Version {
			parsed := ParseVersion(vstr)
			if to.Match(vstr) && (highest == nil || highest.Less(parsed)) {
				highest = &parsed
				result = &block
			}
		}
	}
	return
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

// Verify that there are no semantic errors in the manifest
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
				for _, version := range v.Version {
					if g.Match(ParseVersion(version).String()) {
						found = true
						break
					}
				}
			}
			if !found {
				result = append(result, errors.Errorf("@%s: no version found matching %s", channel.Name, channel.Version))
			}
		}
	}

	return result
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

// mustAbs ensures that "path" is either empty or an absolute file path, after expansion.
func mustAbs(action Action, path string) error {
	if path == "" || filepath.IsAbs(path) {
		return nil
	}
	return participle.Errorf(action.position(), "%q must be an absolute path", path)
}
