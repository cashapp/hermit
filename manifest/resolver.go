package manifest

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/alecthomas/participle/v2"
	"github.com/gobwas/glob"
	"github.com/qdm12/reprint"
	"github.com/tidwall/gjson"

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
	// HTTP client for JSON auto-version requests.
	HTTPClient *http.Client
	platform.Platform
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
	ToPath   string
}

// Package resolved from a manifest.
type Package struct {
	Description          string
	Homepage             string
	Repository           string
	Reference            Reference
	Arch                 string
	Binaries             []string
	Apps                 []string
	Requires             []string
	RuntimeDeps          []Reference
	Provides             []string
	Env                  envars.Ops
	Source               string
	SHA256Source         string
	DontExtract          bool // Don't extract the package, just download it.
	Mirrors              []string
	Root                 string
	SHA256               string
	Mutable              bool
	Dest                 string
	Test                 string
	Strip                int
	Vars                 map[string]string
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
			return nil, errors.Wrapf(err, "%s: no binaries found matching %q - this may indicate a corrupted or misconfigured package, try removing %s and hermit will reinstall it (may require sudo)", p, bin, p.Dest)
		}
		if len(bins) == 0 {
			return nil, errors.Errorf("%s: no binaries found matching %q - this may indicate a corrupted or misconfigured package, try removing %s and hermit will reinstall it (may require sudo)", p, bin, p.Dest)
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
		if !re.MatchString(manifest.Name) && !re.MatchString(manifest.Description) && !re.MatchString(manifest.Homepage) {
			continue
		}
		if len(manifest.Errors) > 0 {
			for _, err := range manifest.Errors {
				l.Warnf("%s:%s", manifest.Path, err)
			}
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
		if len(manifest.Errors) > 0 {
			continue
		}
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
func (r *Resolver) Resolve(l *ui.UI, selector Selector) (*Package, error) {
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

// Resolve a concrete [Package] reference for the given Config.
func Resolve(manifest *AnnotatedManifest, config Config, ref Reference) (*Package, error) {
	return newPackage(manifest, config, ExactSelector(ref))
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
		return nil, errors.Wrapf(ErrUnknownPackage, "%s: no version %s found in versions (%s) or channels (%s), try \"hermit update\"",
			manifest.Path, selector, strings.Join(knownVersions, ", "), strings.Join(knownChannels, ", "))
	}

	root := filepath.Join(config.State, "pkg", found.String())
	p := &Package{
		Description:          manifest.Description,
		Homepage:             manifest.Homepage,
		Repository:           manifest.Repository,
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

	// Find auto-version configuration for JSON variable resolution.
	var autoVersionConfig *AutoVersionBlock
	for _, v := range manifest.Versions {
		for _, version := range v.Version {
			if version == found.Version.String() && v.AutoVersion != nil && v.AutoVersion.JSON != nil {
				autoVersionConfig = v.AutoVersion
				break
			}
		}
		if autoVersionConfig != nil {
			break
		}
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
		if layer.Mutable {
			p.Mutable = layer.Mutable
		}
		if layer.Test != nil {
			p.Test = *layer.Test
		}
		if layer.Source != "" {
			p.Source = layer.Source
		}
		if layer.SHA256Source != "" {
			p.SHA256Source = layer.SHA256Source
		}
		if layer.DontExtract {
			p.DontExtract = layer.DontExtract
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

	home, err := system.UserHomeDir()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Expand variables.
	baseMapping := envars.Mapping(config.Env, home, config.Platform)
	pkgMapping := func(key string) string {
		val := baseMapping(key)
		if val != "" {
			return val
		}

		switch key {
		case "name":
			return found.Name

		case "version":
			return found.Version.String()

		case "dest":
			return layers.field("Dest", p.Dest).(string)

		case "root":
			return layers.field("Root", p.Root).(string)

		default:
			// Handle JSON variable extraction
			if strings.HasPrefix(key, "json:") && autoVersionConfig != nil && autoVersionConfig.JSON != nil && config.HTTPClient != nil {
				jsonPath := key[5:] // Remove "json:" prefix
				if value, err := resolveJSONVariable(config.HTTPClient, autoVersionConfig.JSON, jsonPath); err == nil {
					return value
				}
			}
			// TODO: Should these extra vars go in envars.Mapping?
			return vars[key]
		}
	}

	// Wrap mapping to handle cases where the varable is undefined.
	// weakMapping passes unknown variable references through
	// unaltered.
	weakMapping := func(key string) string {
		val := pkgMapping(key)
		if val == "" {
			return "${" + key + "}"
		}
		return val
	}
	// mapping sets err when unknown variables are found. This error
	// is eventually returned by the current function.
	mapping := func(key string) string {
		val := pkgMapping(key)
		if val == "" {
			err = errors.Errorf("unknown variable $%s", key)
			return ""
		}
		return val
	}

	for _, env := range layerEnvars {
		for k, v := range env {
			// Expand manifest variables but keep other variable references.
			env[k] = envars.Expand(v, weakMapping)
		}
		ops := envars.Infer(env.System())
		// Sort each layer of ops.
		sort.Slice(ops, func(i, j int) bool { return ops[i].Envar() < ops[j].Envar() })
		p.Env = append(p.Env, ops...)
	}
	p.Strip = layers.field("Strip", 0).(int)
	p.Dest = envars.Expand(p.Dest, mapping)
	p.Root = envars.Expand(p.Root, mapping)
	p.Test = envars.Expand(p.Test, mapping)
	for i, bin := range p.Binaries {
		p.Binaries[i] = envars.Expand(bin, mapping)
	}
	for i, requires := range p.Requires {
		p.Requires[i] = envars.Expand(requires, mapping)
	}
	for i, provides := range p.Provides {
		p.Provides[i] = envars.Expand(provides, mapping)
	}
	p.Source = envars.Expand(p.Source, mapping)
	p.SHA256Source = envars.Expand(p.SHA256Source, mapping)
	for i, mirror := range p.Mirrors {
		p.Mirrors[i] = envars.Expand(mirror, mapping)
	}
	// Get sha256 checksum after variable expansion for source, taking care of
	// autoversion
	for _, layer := range layers {
		if layer.SHA256 != "" {
			p.SHA256 = envars.Expand(layer.SHA256, mapping)
		} else if sum, ok := manifest.SHA256Sums[p.Source]; ok {
			p.SHA256 = sum
		}
	}
	inferPackageRepository(p, manifest.Manifest)
	for _, actions := range p.Triggers {
		for _, action := range actions {
			switch action := action.(type) {
			case *RunAction:
				for i, env := range action.Env {
					action.Env[i] = envars.Expand(env, mapping)
				}
				for i, arg := range action.Args {
					action.Args[i] = envars.Expand(arg, mapping)
				}
				action.Command = envars.Expand(action.Command, mapping)
				if err := mustAbs(action, action.Command); err != nil {
					return nil, err
				}
				action.Dir = envars.Expand(action.Dir, mapping)
				if err := mustAbs(action, action.Dir); err != nil {
					return nil, err
				}

			case *CopyAction:
				action.From = envars.Expand(action.From, mapping)
				action.To = envars.Expand(action.To, mapping)
				if err := mustAbs(action, action.To); err != nil {
					return nil, err
				}

			case *ChmodAction:
				action.File = envars.Expand(action.File, mapping)
				if err := mustAbs(action, action.File); err != nil {
					return nil, err
				}

			case *RenameAction:
				action.From = envars.Expand(action.From, mapping)
				if err := mustAbs(action, action.From); err != nil {
					return nil, err
				}
				action.To = envars.Expand(action.To, mapping)
				if err := mustAbs(action, action.To); err != nil {
					return nil, err
				}

			case *DeleteAction:
				for i := range action.Files {
					action.Files[i] = envars.Expand(action.Files[i], mapping)
					if err := mustAbs(action, action.Files[i]); err != nil {
						return nil, err
					}
				}

			case *MessageAction:
				action.Text = envars.Expand(action.Text, mapping)

			case *SymlinkAction:
				action.From = envars.Expand(action.From, mapping)
				action.To = envars.Expand(action.To, mapping)

			case *MkdirAction:
				action.Dir = envars.Expand(action.Dir, mapping)

			default:
				panic(fmt.Sprintf("unsupported action %T", action))
			}
		}
	}
	// This error is set by the mapping() function if ignoreMissing=false and a variable is missing.
	if err != nil {
		return nil, errors.WithStack(err)
	}

	for k, v := range files {
		files[k] = envars.Expand(v, mapping)
	}
	err = resolveFiles(manifest, p, files)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	
	// Populate resolved variables for package
	p.Vars = make(map[string]string)
	for k, v := range vars {
		p.Vars[k] = envars.Expand(v, mapping)
	}
	
	return p, err
}

func inferPackageRepository(p *Package, manifest *Manifest) {
	// start infer from source if no repository is given
	if p == nil || p.Repository != "" || p.Source == "" {
		return
	}

	githubComPrefix := "https://github.com/"

	if manifest != nil {
		for _, v := range manifest.Versions {
			if v.AutoVersion != nil && v.AutoVersion.GitHubRelease != "" {
				p.Repository = fmt.Sprintf("%s%s", githubComPrefix, v.AutoVersion.GitHubRelease)
				return
			}
		}
	}

	if !strings.HasPrefix(p.Source, githubComPrefix) || strings.HasPrefix(p.Source, "https://github.com/cashapp/hermit-build") {
		return
	}

	rest := strings.TrimPrefix(p.Source, githubComPrefix)

	restSplit := strings.Split(rest, "/")

	if len(restSplit) < 2 { //
		return
	}

	result := fmt.Sprintf("%s%s", githubComPrefix, strings.Join(restSplit[0:2], "/"))

	p.Repository = result
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
			ToPath:   v,
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

// resolveJSONVariable fetches JSON data and extracts a value using gjson path syntax.
func resolveJSONVariable(client *http.Client, jsonConfig *JSONAutoVersionBlock, path string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", jsonConfig.URL, nil)
	if err != nil {
		return "", errors.Wrapf(err, "could not create request for JSON variable")
	}

	// Add custom headers if specified
	for key, value := range jsonConfig.Headers {
		req.Header.Set(key, value)
	}

	// Set default Accept header if not specified
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", errors.Wrapf(err, "could not retrieve JSON data")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("%s: HTTP %d", jsonConfig.URL, resp.StatusCode)
	}

	// Read the entire response body for gjson parsing
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "could not read response body")
	}

	// Validate that it's valid JSON
	if !gjson.ValidBytes(body) {
		return "", errors.Errorf("invalid JSON response from %s", jsonConfig.URL)
	}

	// Extract the value using gjson path syntax
	result := gjson.GetBytes(body, path)
	if !result.Exists() {
		return "", errors.Errorf("gjson path query %s matched no results", path)
	}

	if result.Type == gjson.String {
		return result.String(), nil
	}
	// For non-string values, use the raw JSON
	return result.Raw, nil
}
