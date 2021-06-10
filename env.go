package hermit

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/hcl"
	"github.com/kballard/go-shellquote"
	"github.com/pkg/errors"

	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/state"

	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

// Reset environment variables.
var (
	//go:embed files
	files embed.FS

	// UserStateDir should be passed to Open()/Init() in most cases.
	UserStateDir = func() string {
		// Check if state dir is explicitly set
		explicit := os.Getenv("HERMIT_STATE_DIR")
		if explicit != "" {
			return explicit
		}

		cache, err := os.UserCacheDir()
		if err != nil {
			panic(err)
		}
		return filepath.Join(cache, "hermit")
	}()
)

// Files to copy into the env.
var envBinFiles = map[string]os.FileMode{
	"README.hermit.md": 0600,
	"activate-hermit":  0700,
	"hermit":           0700,
}

//go:generate stringer -linecomment -type CleanMask

// CleanMask is a bitmask specifying which parts of a hermit system to clean.
type CleanMask int

// Bitmask for what to clean.
const (
	CleanBin       CleanMask = 1 << iota            // bin
	CleanPackages                                   // packages
	CleanCache                                      // cache
	CleanAll       CleanMask = ^0                   // all
	CleanTransient           = CleanAll &^ CleanBin // transient
)

// Config for a Hermit environment.
type Config struct {
	Envars    envars.Envars `hcl:"env,optional" help:"Extra environment variables."`
	Sources   []string      `hcl:"sources,optional" help:"Package manifest sources."`
	ManageGit bool          `hcl:"manage-git,optional" default:"true" help:"Whether Hermit should automatically 'git add' new packages."`
}

// Env is a Hermit environment.
type Env struct {
	envDir          string
	useGit          bool
	state           *state.State
	binDir          string // Path to bin directory for the environment.
	ephemeralEnvars envars.Ops
	config          *Config
	configFile      string

	// Lazily initialized fields
	lazyResolver *manifest.Resolver
	lazySources  *sources.Sources
}

// Init a new Env.
func Init(l *ui.UI, env string, distURL string, stateDir string, config Config) error {
	env = util.RealPath(env)
	l.Infof("Creating new Hermit environment in %s", env)
	vars := map[string]string{
		"HERMIT_DEFAULT_DIST_URL": distURL,
	}
	bin := filepath.Join(env, "bin")
	if err := os.Mkdir(bin, os.ModePerm); err != nil && !os.IsExist(err) {
		return errors.WithStack(err)
	}
	if err := os.MkdirAll(stateDir, os.ModePerm); err != nil && !os.IsExist(err) {
		return errors.WithStack(err)
	}
	sourcesDir := filepath.Join(stateDir, "sources")
	if err := os.MkdirAll(sourcesDir, os.ModePerm); err != nil && !os.IsExist(err) {
		return errors.WithStack(err)
	}

	useGit := config.ManageGit && isEnvAGitRepo(env)
	b := l.Task(filepath.Base(env))
	for file, perm := range envBinFiles {
		l.Infof("  -> %s", filepath.Join(env, "bin", file))
		if err := writeFileToEnvBin(b, useGit, file, env, vars, perm); err != nil {
			return err
		}
	}

	// Create a configuration file.
	configPath := filepath.Join(bin, "hermit.hcl")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		l.Infof("  -> %s", configPath)
		data, err := hcl.Marshal(&config)
		if err != nil {
			return errors.WithStack(err)
		}
		err = ioutil.WriteFile(configPath, data, 0600)
		if err != nil {
			return errors.WithStack(err)
		}
		if useGit {
			if err = util.RunInDir(b, env, "git", "add", "-f", filepath.Join(bin, "hermit.hcl")); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	l.Infof(`

Hermit environment initialised in %s

To activate the environment run:

  . %s/bin/activate-hermit

Then run the following to list available commands:

  hermit --help

To deactivate the environment run:

  deactivate-hermit

For more information please refer to https://github.com/cashapp/hermit
`, env, env)
	return nil
}

// EnvDirFromProxyLink finds a Hermit environment given a proxy symlink.
func EnvDirFromProxyLink(executable string) (string, error) {
	links, err := util.ResolveSymlinks(executable)
	if err != nil {
		return "", errors.WithStack(err)
	}
	last := links[len(links)-1]
	if filepath.Base(last) != "hermit" {
		return "", errors.Errorf("binary is not a Hermit symlink: %s", links[0])
	}
	envDir := filepath.Dir(filepath.Dir(last))
	return envDir, nil
}

func getSources(l *ui.UI, envDir string, config *Config, state *state.State, defaultSources []string) (*sources.Sources, error) {
	configuredSources := config.Sources
	if config.Sources == nil {
		configuredSources = defaultSources
	}
	ss, err := sources.ForURIs(l, state.SourcesDir(), envDir, configuredSources)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// Always include the builtin sources required by Hermit.
	ss.Prepend(state.Config().Builtin)
	return ss, nil
}

func readConfig(configFile string) (*Config, error) {
	config := &Config{Envars: map[string]string{}, ManageGit: true}
	source, err := ioutil.ReadFile(configFile)
	if !os.IsNotExist(err) {
		if err != nil {
			return nil, errors.Wrap(err, "couldn't load environment config")
		}
		err = hcl.Unmarshal(source, config)
		if err != nil {
			return nil, errors.Wrap(err, configFile)
		}
	}
	return config, nil
}

// OpenEnv opens a Hermit environment.
//
// The environment may not exist, in which case this will succeed but subsequent operations will fail.
func OpenEnv(envDir string, state *state.State, ephemeral envars.Envars) (*Env, error) {
	binDir := filepath.Join(envDir, "bin")
	configFile := filepath.Join(binDir, "hermit.hcl")
	config, err := readConfig(configFile)
	if err != nil {
		return nil, errors.Wrap(err, configFile)
	}

	useGit := config.ManageGit && isEnvAGitRepo(envDir)
	envDir = util.RealPath(envDir)

	e := &Env{
		config:          config,
		envDir:          envDir,
		useGit:          useGit,
		state:           state,
		binDir:          binDir,
		configFile:      configFile,
		ephemeralEnvars: envars.Infer(ephemeral.System()),
	}
	return e, nil
}

// Root directory of the environment.
func (e *Env) Root() string {
	return e.envDir
}

// Trigger an event.
func (e *Env) Trigger(l *ui.UI, event manifest.Event) (messages []string, err error) {
	pkgs, err := e.ListInstalled(l)
	if err != nil {
		return nil, err
	}
	for _, pkg := range pkgs {
		eventMessages, err := pkg.Trigger(l, event)
		if err != nil {
			return nil, errors.Wrapf(err, "%s: on %s", pkg, event)
		}
		messages = append(messages, eventMessages...)
	}
	return messages, nil
}

// ValidateManifests from all sources.
func (e *Env) ValidateManifests(l *ui.UI) (manifest.ManifestErrors, error) {
	resolver, err := e.resolver(l)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if err := resolver.LoadAll(); err != nil {
		return nil, errors.WithStack(err)
	}
	return resolver.Errors(), nil
}

// GC can be used to clean up unused packages, and clear the download cache.
func (e *Env) GC(l *ui.UI, age time.Duration) error {
	return e.state.GC(l, age, e.Resolve)
}

// LinkedBinaries lists just the binaries installed in the environment.
func (e *Env) LinkedBinaries(pkg *manifest.Package) (binaries []string, err error) {
	files, err := ioutil.ReadDir(e.binDir)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, file := range files {
		bin := filepath.Join(e.binDir, file.Name())
		link, err := os.Readlink(bin)
		if err != nil || !strings.HasSuffix(link, ".pkg") {
			continue
		}
		ref := e.referenceFromBinLink(link)
		if ref.String() == pkg.String() {
			binaries = append(binaries, bin)
		}
	}
	return
}

// Uninstall uninstalls a single package.
func (e *Env) Uninstall(l *ui.UI, pkg *manifest.Package) (*shell.Changes, error) {
	return e.uninstall(l.Task(pkg.Reference.String()), pkg)
}

func (e *Env) uninstall(l *ui.Task, pkg *manifest.Package) (*shell.Changes, error) {
	log := l.SubTask("uninstall")
	log.Infof("Uninstalling %s", pkg)
	// Is it installed?
	link := e.pkgLink(pkg)
	if _, err := os.Stat(link); os.IsNotExist(err) {
		return nil, errors.Errorf("package %s is not installed", pkg)
	}

	ops := e.envarsForPackages(pkg)

	// Remove symlinks in the bin dir.
	err := e.unlinkPackage(l, pkg)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	changes := shell.NewChanges(envars.Parse(os.Environ()))
	changes.Remove = ops

	err = e.state.RecordUninstall(pkg, e.binDir)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return changes, nil
}

func (e *Env) unlinkPackage(l *ui.Task, pkg *manifest.Package) error {

	link := e.pkgLink(pkg)

	binaries, err := e.LinkedBinaries(pkg)
	if err != nil {
		return errors.WithStack(err)
	}

	task := l.SubProgress("unlink", 1+len(binaries))
	defer task.Done()
	task.Debugf("Uninstalling %s", pkg)

	err = e.unlink(task, link)
	if err != nil && !os.IsNotExist(err) {
		return errors.WithStack(err)
	}

	task.Add(1)

	for _, link := range binaries {
		err := e.unlink(task, link)
		task.Add(1)
		if err != nil && !os.IsNotExist(err) {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (e *Env) unlink(l *ui.Task, path string) error {
	if e.useGit {
		err := util.RunInDir(l, e.envDir, "git", "rm", "-f", path)
		if err != nil {
			l.Errorf("non-fatal: %s", err)
		}
	}
	l.Tracef("rm %s", path)
	return os.Remove(path)
}

// Test the sanity of a package using the configured test shell fragment.
func (e *Env) Test(l *ui.UI, pkg *manifest.Package) error {
	task := l.Task(pkg.Reference.String())
	if pkg.Test == "" {
		return nil
	}
	args, err := shellquote.Split(pkg.Test)
	if err != nil {
		return errors.Wrapf(err, "%s: invalid test shell fragment %q", pkg.String(), pkg.Test)
	}
	if err = e.state.CacheAndUnpack(task, pkg); err != nil {
		return errors.WithStack(err)
	}
	bins, err := pkg.ResolveBinaries()
	if err != nil {
		return errors.WithStack(err)
	}
	found := false
	for _, bin := range bins {
		if filepath.Base(bin) == args[0] {
			args[0] = bin
			found = true
			break
		}
	}
	if !found {
		return errors.Errorf("couldn't find test executable %q in package %s", args[0], pkg)
	}
	cmd, _ := util.Command(task, args...)
	cmd.Env = e.allEnvarsForPackages(true, pkg)
	return cmd.Run()
}

// Unpack but do not install package.
func (e *Env) Unpack(l *ui.Task, p *manifest.Package) error {
	task := l.SubTask(p.Reference.String())
	return e.state.CacheAndUnpack(task, p)
}

// Install package. If a package with same name exists, uninstall it first.
func (e *Env) Install(l *ui.UI, pkg *manifest.Package) (*shell.Changes, error) {
	task := l.Task(pkg.Reference.String())

	installed, err := e.ListInstalled(l)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	allChanges := shell.NewChanges(envars.Parse(os.Environ()))

	for _, ipkg := range installed {
		if ipkg.Reference.Name == pkg.Reference.Name {
			changes, err := e.uninstall(task, ipkg)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			allChanges = allChanges.Merge(changes)
		}
	}

	changes, err := e.install(task, pkg)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return allChanges.Merge(changes), nil
}

// install a package
func (e *Env) install(l *ui.Task, p *manifest.Package) (*shell.Changes, error) {
	p.UpdatedAt = time.Now()
	log := l.SubTask("install")
	log.Infof("Installing %s", p)
	log.Debugf("From %s", p.Source)
	log.Debugf("To %s", p.Dest)
	err := e.state.CacheAndUnpack(l, p)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if _, err := os.Stat(e.pkgLink(p)); os.IsNotExist(err) {
		if err = e.linkPackage(l, p); err != nil {
			return nil, errors.WithStack(err)
		}
		if err = e.state.WritePackageState(p, e.binDir); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	ops := e.envarsForPackages(p)
	changes := shell.NewChanges(envars.Parse(os.Environ()))
	changes.Add = ops

	return changes, nil
}

// Upgrade package.
func (e *Env) Upgrade(l *ui.UI, pkg *manifest.Package) (*shell.Changes, error) {
	task := l.Task(pkg.Reference.String())

	if pkg.Reference.IsChannel() {
		return nil, e.upgradeChannel(task, pkg)
	}
	return e.upgradeVersion(l, pkg)
}

// ResolveLink returns the package for a hermit bin dir link.
//
// Link chains are in the form
//
//     <binary> -> <pkg>-<version>.pkg -> hermit
func (e *Env) ResolveLink(l *ui.UI, executable string) (pkg *manifest.Package, binary string, err error) {
	links, err := util.ResolveSymlinks(executable)
	if err != nil {
		return nil, "", errors.WithStack(err)
	}
	var (
		link  string
		found bool
	)
	for _, link = range links {
		if strings.HasSuffix(link, ".pkg") {
			found = true
			break
		}
		binary = link
	}
	if !found {
		return nil, "", errors.Errorf("%s: could not find Hermit .pkg in symlink chain", executable)
	}
	ref := e.referenceFromBinLink(link)
	pkg, err = e.Resolve(l, manifest.ExactSelector(ref), true)
	if err != nil {
		return nil, "", errors.WithStack(err)
	}
	return pkg, binary, nil
}

// Exec the specified binary from the given package, replacing this process.
//
// "args" should be os.Args (or equivalent), including the binary name.
// "deps" contains all packages that need to be in the system for the execution.
// The missing dependencies are downloaded and unpacked.
func (e *Env) Exec(l *ui.UI, pkg *manifest.Package, binary string, args []string, deps map[string]*manifest.Package) error {
	b := l.Task(pkg.Reference.String())
	timer := ui.LogElapsed(l, "exec")
	for _, dep := range deps {
		err := e.state.CacheAndUnpack(l.Task(dep.Reference.String()), dep)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	err := e.EnsureChannelIsUpToDate(l, pkg)
	if err != nil {
		return errors.WithStack(err)
	}
	binaries, err := pkg.ResolveBinaries()
	if err != nil {
		return errors.WithStack(err)
	}
	env, err := e.Envars(l, true)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, bin := range binaries {
		if filepath.Base(bin) != filepath.Base(binary) {
			continue
		}
		err = e.state.WritePackageState(pkg, e.binDir)
		if err != nil {
			return errors.WithStack(err)
		}
		b.Tracef("exec %s", shellquote.Join(append([]string{bin}, args...)...))
		l.Clear()
		timer()

		err = syscall.Exec(bin, args, env)
		return errors.Wrapf(err, "%s: failed to execute %q", pkg, bin)
	}
	return errors.Errorf("%s: could not find binary %q", pkg, binary)
}

// Resolve package reference.
//
// If "syncOnMissing" is true, sources will be synced if the selector cannot
// be initially resolved, then resolve attempted again.
func (e *Env) Resolve(l *ui.UI, selector manifest.Selector, syncOnMissing bool) (*manifest.Package, error) {
	resolver, err := e.resolver(l)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	resolved, err := resolver.Resolve(l, selector)
	// If the package is missing sync sources and try again, once.
	if syncOnMissing && errors.Is(err, manifest.ErrUnknownPackage) {
		if err = resolver.Sync(l, true); err != nil {
			return nil, errors.WithStack(err)
		}
		resolved, err = resolver.Resolve(l, selector)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}

	e.readPackageState(resolved)
	return resolved, nil
}

// ResolveVirtual references to concrete packages.
func (e *Env) ResolveVirtual(l *ui.UI, name string) ([]*manifest.Package, error) {
	resolver, err := e.resolver(l)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	resolved, err := resolver.ResolveVirtual(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, pkg := range resolved {
		e.readPackageState(pkg)
	}
	return resolved, nil
}

// UpdateUsage updates the package usage time stamps in the underlying database.
// if the package was not previously present, it is inserted to the DB.
func (e *Env) UpdateUsage(pkg *manifest.Package) error {
	err := e.state.WritePackageState(pkg, e.binDir)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ListInstalledReferences from this environment.
func (e *Env) ListInstalledReferences() ([]manifest.Reference, error) {
	matches, err := filepath.Glob(filepath.Join(e.binDir, ".*.pkg"))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	sort.Strings(matches)
	out := []manifest.Reference{}
	for _, pkgLink := range matches {
		ref := e.referenceFromBinLink(pkgLink)
		out = append(out, ref)
	}
	return out, nil
}

// ListInstalled packages from this environment.
func (e *Env) ListInstalled(l *ui.UI) ([]*manifest.Package, error) {
	refs, err := e.ListInstalledReferences()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	out := []*manifest.Package{}
	for _, ref := range refs {
		pkg, err := e.Resolve(l, manifest.ExactSelector(ref), false)
		if err != nil { // We don't want to error if there are corrupt packages.
			continue
		}
		out = append(out, pkg)
	}
	return out, nil
}

// Envars returns the fully expanded envars for this environment.
//
// PATH, HERMIT_BIN and HERMIT_ENV will always be explicitly set, plus all
// environment variables defined in the packages installed in the environment,
// and finally any environment variables explicitly configured in the environment.
//
// If "inherit" is true, variables from the shell environment will be inherited. Otherwise
// only variables defined in Hermit itself will be available.
func (e *Env) Envars(l *ui.UI, inherit bool) ([]string, error) {
	defer ui.LogElapsed(l, "envars")()
	pkgs, err := e.ListInstalled(l)
	if err != nil {
		return nil, err
	}
	return e.allEnvarsForPackages(inherit, pkgs...), nil
}

// EnvOps returns the envar mutation operations for this environment.
//
// PATH, HERMIT_BIN and HERMIT_ENV will always be explicitly set, plus all
// environment variables defined in the packages installed in the environment,
// and finally any environment variables explicitly configured in the environment.
func (e *Env) EnvOps(l *ui.UI) (envars.Ops, error) {
	pkgs, err := e.ListInstalled(l)
	if err != nil {
		return nil, err
	}
	ops := e.envarsForPackages(pkgs...)
	ops = append(ops, e.hermitEnvarOps()...)
	ops = append(ops, e.localEnvarOps()...)
	ops = append(ops, e.ephemeralEnvars...)
	return ops, nil
}

// SetEnv sets an extra environment variable.
func (e *Env) SetEnv(key, value string) error {
	e.config.Envars[key] = value
	data, err := hcl.Marshal(e.config)
	if err != nil {
		return errors.WithStack(err)
	}
	return ioutil.WriteFile(e.configFile, data, 0600)
}

// DelEnv deletes a custom environment variable.
func (e *Env) DelEnv(key string) error {
	delete(e.config.Envars, key)
	data, err := hcl.Marshal(e.config)
	if err != nil {
		return errors.WithStack(err)
	}
	return ioutil.WriteFile(e.configFile, data, 0600)
}

// Clean parts of the hermit system.
func (e *Env) Clean(l *ui.UI, level CleanMask) error {
	if level&CleanBin != 0 {
		pkgs, err := e.ListInstalled(l)
		if err != nil {
			return err
		}
		for _, pkg := range pkgs {
			b := l.Task("clean")
			bins, err := pkg.ResolveBinaries()
			if err != nil {
				return errors.WithStack(err)
			}
			for _, bin := range bins {
				bin = filepath.Join(e.binDir, filepath.Base(bin))
				b.Debugf("rm -f %q", bin)
				_ = os.Remove(bin)
			}
			b.Debugf("rm -f %q", e.pkgLink(pkg))
			_ = os.Remove(e.pkgLink(pkg))
		}
	}
	if level&CleanPackages != 0 {
		err := e.state.CleanPackages(l)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	if level&CleanCache != 0 {
		err := e.state.CleanCache(l)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// Search for packages using the given regular expression.
func (e *Env) Search(l *ui.UI, pattern string) (manifest.Packages, error) {
	resolver, err := e.resolver(l)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	pkgs, err := resolver.Search(l, pattern)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, pkg := range pkgs {
		e.readPackageState(pkg)
	}
	return pkgs, nil
}

// EnsureChannelIsUpToDate updates the package if it has an update interval,
// the required time since the last update check has passed,
// and the etag in the source has changed from the last check.
//
// This should only be called for packages that have already been installed
func (e *Env) EnsureChannelIsUpToDate(l *ui.UI, pkg *manifest.Package) error {
	if pkg.UpdateInterval == 0 || pkg.UpdatedAt.After(time.Now().Add(-1*pkg.UpdateInterval)) {
		// No updates needed for this package
		return nil
	}

	return e.upgradeChannel(l.Task(pkg.Reference.String()), pkg)
}

// AddSource adds a new source bundle and refreshes the packages from it
func (e *Env) AddSource(l *ui.UI, s sources.Source) error {
	sources, err := e.sources(l)
	if err != nil {
		return errors.WithStack(err)
	}
	sources.Add(s)
	return e.Sync(l, true)
}

// EnvDir returns the directory where this environment is rooted
func (e *Env) EnvDir() string {
	return e.envDir
}

// BinDir returns the directory for the binaries of this environment
func (e *Env) BinDir() string {
	return e.binDir
}

func (e *Env) upgradeChannel(task *ui.Task, pkg *manifest.Package) error {
	task.Infof("Upgrading %s", pkg)
	_, err := e.state.UpgradeChannel(task, pkg)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// upgradeVersion upgrades the package to its latest version.
// If the package is already at its latest version, this is a no-op.
func (e *Env) upgradeVersion(l *ui.UI, pkg *manifest.Package) (*shell.Changes, error) {
	resolver, err := e.resolver(l)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// Get the latest version of the package
	resolved, err := resolver.Resolve(l, manifest.PrefixSelector(pkg.Reference.Major()))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if !resolved.Reference.Version.Match(pkg.Reference.Version) {
		l.Task(pkg.Reference.Name).SubTask("upgrade").Infof("Upgrading %s to %s", pkg, resolved)
		uc, err := e.uninstall(l.Task(pkg.Reference.String()), pkg)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		ic, err := e.install(l.Task(resolved.Reference.String()), resolved)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return uc.Merge(ic), nil
	}
	return shell.NewChanges(envars.Parse(os.Environ())), nil
}

func (e *Env) readPackageState(pkg *manifest.Package) {
	_, err := os.Stat(e.pkgLink(pkg))
	pkg.Linked = err == nil
	e.state.ReadPackageState(pkg)
}

func (e *Env) referenceFromBinLink(pkgLink string) manifest.Reference {
	name := strings.TrimPrefix(strings.TrimSuffix(filepath.Base(pkgLink), ".pkg"), ".")
	return manifest.ParseReference(name)
}

func (e *Env) checkForConflicts(files []string, pkg *manifest.Package) error {
	conflicts := []string{}
	for _, file := range files {
		link := filepath.Join(e.binDir, filepath.Base(file))
		if _, err := os.Lstat(link); !os.IsNotExist(err) {
			if err != nil {
				return errors.WithStack(err)
			}
			conflicts = append(conflicts, filepath.Base(file))
		}
	}
	if len(conflicts) > 0 {
		joined := strings.Join(conflicts, ", ")
		return errors.Errorf("%s can not be installed, the following binaries already exist: %s", pkg.String(), joined)
	}
	return nil
}

func (e *Env) linkPackage(l *ui.Task, pkg *manifest.Package) error {
	task := l.SubTask("link")
	files, err := pkg.ResolveBinaries()
	if err != nil {
		return errors.WithStack(err)
	}
	err = e.checkForConflicts(files, pkg)
	if err != nil {
		return err
	}
	task.Debugf("Linking binaries for %s", pkg)
	// Add package link.
	pkgLink := e.pkgLink(pkg)
	task.Size(len(files) + 1)
	defer task.Done()
	task.Add(1)
	err = e.linkIntoEnv(task, "hermit", pkgLink)
	if err != nil {
		return errors.Wrapf(err, "failed to create binary link %s", util.RelPathCWD(pkgLink))
	}
	for _, file := range files {
		task.Add(1)
		link := filepath.Join(e.binDir, filepath.Base(file))
		err = e.linkIntoEnv(task, filepath.Base(pkgLink), link)
		if err != nil {
			return errors.Wrapf(err, "failed to create binary link %s", util.RelPathCWD(link))
		}
	}
	for _, app := range pkg.Apps {
		err = e.linkApp(app)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Env) linkIntoEnv(l *ui.Task, oldname, newname string) error {
	l.Debugf("ln -s %q %q", oldname, newname)
	if err := os.Symlink(oldname, newname); err != nil {
		return errors.WithStack(err)
	}
	if e.useGit {
		return util.RunInDir(l, e.envDir, "git", "add", "-f", newname)
	}
	return nil
}

func (e *Env) pkgLink(pkg *manifest.Package) string {
	return filepath.Join(e.binDir, fmt.Sprintf(".%s.pkg", pkg))
}

// Returns combined system + Hermit + package environment variables, fully expanded.
//
// If "inherit" is true, system envars will be included.
func (e *Env) allEnvarsForPackages(inherit bool, pkgs ...*manifest.Package) []string {
	var ops envars.Ops
	system := envars.Parse(os.Environ())
	ops = append(ops, e.envarsForPackages(pkgs...)...)
	ops = append(ops, e.localEnvarOps()...)
	ops = append(ops, e.hermitEnvarOps()...)
	ops = append(ops, e.ephemeralEnvars...)
	transform := system.Apply(e.Root(), ops)
	if inherit {
		return transform.Combined().System()
	}
	return transform.Changed(false).System()
}

// envarsForPackages returns the environment variable operations by the given packages.
func (e *Env) envarsForPackages(pkgs ...*manifest.Package) envars.Ops {
	out := envars.Ops{}
	for _, pkg := range pkgs {
		out = append(out, pkg.Env...)
	}
	return out
}

// localEnvarOps returns the environment variables defined in the local configuration
func (e *Env) localEnvarOps() envars.Ops {
	return envars.Infer(e.config.Envars.System())
}

// hermitEnvarOps returns the environment variables created and reuqired by hermit itself
func (e *Env) hermitEnvarOps() envars.Ops {
	return envars.Ops{
		&envars.Prepend{Name: "PATH", Value: e.binDir},
		&envars.Force{Name: "HERMIT_BIN", Value: e.binDir},
		&envars.Force{Name: "HERMIT_ENV", Value: e.envDir},
	}
}

func (e *Env) linkApp(app string) error {
	root := filepath.Join(e.binDir, filepath.Base(app))
	for _, dir := range []string{"Contents/MacOS", "Contents/Resources"} {
		err := os.MkdirAll(filepath.Join(root, dir), 0700)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// Sync sources.
func (e *Env) Sync(l *ui.UI, force bool) error {
	resolver, err := e.resolver(l)
	if err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(resolver.Sync(l, force))
}

// Sources enabled in this environment.
func (e *Env) Sources(l *ui.UI) ([]string, error) {
	sources, err := e.sources(l)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return sources.Sources(), nil
}

// ResolveWithDeps collect packages and their dependencies based on the given manifest.Selector into a map
func (e *Env) ResolveWithDeps(l *ui.UI, installed manifest.Packages, selector manifest.Selector, out map[string]*manifest.Package) (err error) {
	for _, existing := range installed {
		if existing.Reference.String() == selector.Name() {
			return nil
		}
	}
	pkg, err := e.Resolve(l, selector, false)
	if err != nil {
		return errors.WithStack(err)
	}
	out[pkg.Reference.String()] = pkg
	for _, req := range pkg.Requires {
		// First search from virtual providers
		ref, err := e.resolveVirtual(l, req)
		if err != nil && errors.Is(err, manifest.ErrUnknownPackage) {
			// Secondly search by the package name
			return e.ResolveWithDeps(l, installed, manifest.NameSelector(req), out)
		}
		if err != nil {
			return err
		}
		err = e.ResolveWithDeps(l, installed, manifest.ExactSelector(ref), out)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (e *Env) resolveVirtual(l *ui.UI, name string) (manifest.Reference, error) {
	virtual, err := e.ResolveVirtual(l, name)
	if err != nil {
		return manifest.Reference{}, errors.WithStack(err)
	}
	installed, err := e.ListInstalledReferences()
	if err != nil {
		return manifest.Reference{}, errors.WithStack(err)
	}
	candidates := []string{}
	for _, vpkg := range virtual {
		candidates = append(candidates, vpkg.Reference.Name)
		for _, ref := range installed {
			if ref.Name == vpkg.Reference.Name {
				return ref, nil
			}
		}
	}
	return manifest.Reference{}, errors.Errorf("multiple packages satisfy the required dependency %q, please install one of the following manually: %s", name, strings.Join(candidates, ", "))
}

func isEnvAGitRepo(env string) bool {
	_, err := os.Stat(filepath.Join(env, ".git"))
	return err == nil
}

// Write a file from the bundled VFS to the env bin dir, performing basic variable substitution.
func writeFileToEnvBin(l *ui.Task, useGit bool, src, envDir string, vars map[string]string, perm os.FileMode) error {
	dest := filepath.Join(envDir, "bin", src)
	source, err := fs.ReadFile(files, filepath.Join("files", src))
	if err != nil {
		return errors.Wrap(err, src)
	}
	for key, value := range vars {
		source = bytes.ReplaceAll(source, []byte(key), []byte(value))
	}
	if err = ioutil.WriteFile(dest, source, perm); err != nil {
		return errors.WithStack(err)
	}
	if useGit {
		if err = util.RunInDir(l, envDir, "git", "add", "-f", dest); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (e *Env) sources(l *ui.UI) (*sources.Sources, error) {
	if e.lazySources != nil {
		return e.lazySources, nil
	}
	sources, err := getSources(l, e.envDir, e.config, e.state, e.state.Config().Sources)
	if err != nil {
		return nil, errors.Wrap(err, e.configFile)
	}
	e.lazySources = sources
	return sources, nil
}

func (e *Env) resolver(l *ui.UI) (*manifest.Resolver, error) {
	if e.lazyResolver != nil {
		return e.lazyResolver, nil
	}
	sources, err := e.sources(l)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	resolver, err := manifest.New(sources, manifest.Config{
		Env:   e.envDir,
		State: e.state.Root(),
		OS:    runtime.GOOS,
		Arch:  runtime.GOARCH,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	e.lazyResolver = resolver
	return resolver, nil
}
