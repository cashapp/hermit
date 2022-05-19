package hermit

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/hcl"
	"github.com/kballard/go-shellquote"

	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/internal/system"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/platform"
	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

// Reset environment variables.
var (
	//go:embed files/script.sha256
	sha256Db string

	// ScriptSHAs contains the default known valid SHA256 sums for bin/activate-hermit and bin/hermit.
	ScriptSHAs = tidySha256Db(sha256Db)

	//go:embed files
	files embed.FS

	// UserStateDir should be passed to Open()/Init() in most cases.
	UserStateDir = func() string {
		// Check if state dir is explicitly set
		explicit := os.Getenv("HERMIT_STATE_DIR")
		if explicit != "" {
			return explicit
		}
		cache, err := system.UserCacheDir()
		if err != nil {
			panic(fmt.Sprintf("could not find user cache dir: %s", err))
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
	Envars      envars.Envars `hcl:"env,optional" help:"Extra environment variables."`
	Sources     []string      `hcl:"sources,optional" help:"Package manifest sources."`
	ManageGit   bool          `hcl:"manage-git,optional" default:"true" help:"Whether Hermit should automatically 'git add' new packages."`
	AddIJPlugin bool          `hcl:"idea,optional" default:"false" help:"Whether Hermit should automatically add the IntelliJ IDEA plugin."`
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
	httpClient      *http.Client
	scriptSums      []string

	// Lazily initialized fields
	lazyResolver  *manifest.Resolver
	lazySources   *sources.Sources
	packageSource cache.PackageSourceSelector
}

//go:embed files/externalDependencies.xml
var extDepData []byte

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

	if config.AddIJPlugin {
		ideaPath := filepath.Join(env, ".idea")
		extDepPath := filepath.Join(ideaPath, "externalDependencies.xml")

		if _, err := os.Stat(extDepPath); errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(ideaPath, os.ModePerm); err != nil && !os.IsExist(err) {
				return errors.WithStack(err)
			}

			err = ioutil.WriteFile(extDepPath, extDepData, 0600)
			if err != nil {
				return errors.WithStack(err)
			}

			if useGit {
				if err = util.RunInDir(b, env, "git", "add", "-f", extDepPath); err != nil {
					return errors.WithStack(err)
				}
			}
		} else {
			l.Infof("IntelliJ IDEA configuration already exists; skipping adding configuration.")
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
	last, err := filepath.EvalSymlinks(links[len(links)-1])
	if err != nil {
		return "", errors.WithStack(err)
	}
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
//
// "scriptSums" contains all known SHA256 checksums for "bin/hermit" and "bin/activate-hermit" scripts.
func OpenEnv(
	envDir string,
	state *state.State,
	packageSource cache.PackageSourceSelector,
	ephemeral envars.Envars,
	httpClient *http.Client,
	scriptSums []string,
) (*Env, error) {
	binDir := filepath.Join(envDir, "bin")
	configFile := filepath.Join(binDir, "hermit.hcl")
	config, err := readConfig(configFile)
	if err != nil {
		return nil, errors.Wrap(err, configFile)
	}

	useGit := config.ManageGit && isEnvAGitRepo(envDir)
	envDir = util.RealPath(envDir)
	if len(scriptSums) == 0 {
		scriptSums = ScriptSHAs
	}

	return &Env{
		packageSource:   packageSource,
		config:          config,
		envDir:          envDir,
		useGit:          useGit,
		state:           state,
		binDir:          binDir,
		configFile:      configFile,
		ephemeralEnvars: envars.Infer(ephemeral.System()),
		httpClient:      httpClient,
		scriptSums:      scriptSums,
	}, nil
}

// Root directory of the environment.
func (e *Env) Root() string {
	return e.envDir
}

// Verify contains valid Hermit scripts.
func (e *Env) Verify() error {
next:
	for _, path := range []string{"activate-hermit", "hermit"} {
		path = filepath.Join(e.binDir, path)
		hasher := sha256.New()
		r, err := os.Open(path)
		if os.IsNotExist(err) {
			return errors.Wrapf(err, "%s is missing, not a Hermit environment?", path)
		} else if err != nil {
			return errors.WithStack(err)
		}
		_, err = io.Copy(hasher, r)
		_ = r.Close()
		if err != nil {
			return errors.WithStack(err)
		}
		hash := hex.EncodeToString(hasher.Sum(nil))
		for _, candidate := range e.scriptSums {
			if hash == candidate {
				continue next
			}
		}
		return errors.Errorf("%s has an unknown SHA256 signature (%s); verify that you trust this environment and run 'hermit init %s'", path, hash, e.envDir)
	}
	return nil
}

// Trigger an event for all installed packages.
func (e *Env) Trigger(l *ui.UI, event manifest.Event) (messages []string, err error) {
	pkgs, err := e.ListInstalled(l)
	if err != nil {
		return nil, err
	}
	for _, pkg := range pkgs {
		pkgMessages, err := e.TriggerForPackage(l, event, pkg)
		if err != nil {
			return nil, err
		}
		messages = append(messages, pkgMessages...)
	}
	return messages, nil
}

// TriggerForPackage triggers an event for a single package.
func (e *Env) TriggerForPackage(l *ui.UI, event manifest.Event, pkg *manifest.Package) (messages []string, err error) {
	messages, err = pkg.Trigger(l, event)
	if err != nil {
		return nil, errors.Wrapf(err, "%s: on %s", pkg, event)
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
	cmd, out := util.Command(task, args...)
	deps, err := e.ensureRuntimeDepsPresent(l, pkg)
	if err != nil {
		return errors.WithStack(err)
	}
	cmd.Env = e.envarsFromOps(true, e.allEnvarOpsForPackages(deps, pkg))
	err = cmd.Run()
	if err != nil {
		return errors.Wrap(err, out.String())
	}
	return nil
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
	if err := pkg.EnsureSupported(); err != nil {
		return nil, errors.Wrapf(err, "install failed")
	}

	allChanges := shell.NewChanges(envars.Parse(os.Environ()))

	didUninstall := false
	for _, ipkg := range installed {
		if ipkg.Reference.Name == pkg.Reference.Name {
			changes, err := e.uninstall(task, ipkg)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			allChanges = allChanges.Merge(changes)
			didUninstall = true
		}
	}

	if !didUninstall && len(pkg.UnsupportedPlatforms) > 0 {

		resp, err := l.Confirmation("%s is not supported on these Hermit platforms: %s, are you sure you want to install it? [y/N]", pkg.Reference, pkg.UnsupportedPlatforms)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if !resp {
			return allChanges, nil
		}
	}

	changes, err := e.install(l, pkg)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return allChanges.Merge(changes), nil
}

// resolveRuntimeDependencies checks all runtime dependencies for a package are available.
//
// Aggregate and collect the package names and binaries of all runtime dependencies to avoid collisions.
func (e *Env) resolveRuntimeDependencies(l *ui.UI, p *manifest.Package, aggregate map[string]*manifest.Package, bins map[string]*manifest.Package) error {
	var depPkgs []*manifest.Package
	// Explicitly specified runtime-dependencies in the package.
	for _, ref := range p.RuntimeDeps {
		previous := aggregate[ref.Name]
		if previous != nil && previous.Reference.Compare(ref) != 0 {
			return errors.Errorf("two conflicting runtime-dependencies: %s vs %s", ref, previous.Reference)
		}

		depPkg, err := e.Resolve(l, manifest.ExactSelector(ref), true)
		if err != nil {
			return errors.WithStack(err)
		}

		depPkgs = append(depPkgs, depPkg)
	}

	for _, depPkg := range depPkgs {
		for _, bin := range depPkg.Binaries {
			base := filepath.Base(bin)
			if previous, ok := bins[base]; ok && !previous.Reference.Match(depPkg.Reference) {
				return errors.Errorf("conflicting binary %q in multiple runtime dependencies: %s and %s", bin, previous.Reference, depPkg.Reference)
			}
			bins[base] = depPkg
		}

		aggregate[depPkg.Reference.Name] = depPkg
		if err := e.resolveRuntimeDependencies(l, depPkg, aggregate, bins); err != nil {
			return err
		}
	}

	return nil
}

func (e *Env) ensureRuntimeDepsPresent(l *ui.UI, p *manifest.Package) ([]*manifest.Package, error) {
	deps := map[string]*manifest.Package{}
	err := e.resolveRuntimeDependencies(l, p, deps, map[string]*manifest.Package{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	result := make([]*manifest.Package, 0, len(deps))
	for _, pkg := range deps {
		if err := e.state.CacheAndUnpack(l.Task(p.Reference.String()), pkg); err != nil {
			return nil, errors.WithStack(err)
		}
		result = append(result, pkg)
	}
	return result, nil
}

// Update timestamps for runtime dependencies.
func (e *Env) writePackageState(pkgs ...*manifest.Package) error {
	for _, pkg := range pkgs {
		if err := e.state.WritePackageState(pkg); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// install a package
func (e *Env) install(l *ui.UI, p *manifest.Package) (*shell.Changes, error) {
	task := l.Task(p.Reference.String())

	pkgs, err := e.ensureRuntimeDepsPresent(l, p)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	p.UpdatedAt = time.Now()
	log := task.SubTask("install")
	log.Infof("Installing %s", p)
	log.Debugf("From %s", p.Source)
	log.Debugf("To %s", p.Dest)
	err = e.state.CacheAndUnpack(task, p)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if _, err := os.Stat(e.pkgLink(p)); os.IsNotExist(err) {
		if err = e.linkPackage(task, p); err != nil {
			return nil, errors.WithStack(err)
		}
		pkgs = append(pkgs, p)
	}
	ops := e.envarsForPackages(p)
	changes := shell.NewChanges(envars.Parse(os.Environ()))
	changes.Add = ops

	return changes, errors.WithStack(e.writePackageState(pkgs...))
}

// Upgrade package.
func (e *Env) Upgrade(l *ui.UI, pkg *manifest.Package) (*shell.Changes, error) {
	task := l.Task(pkg.Reference.String())

	if pkg.Reference.IsChannel() {
		err := e.state.UpgradeChannel(task, pkg)
		return nil, errors.WithStack(err)
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
//
// The missing dependencies are downloaded and unpacked.
func (e *Env) Exec(l *ui.UI, pkg *manifest.Package, binary string, args []string, deps map[string]*manifest.Package) error {
	b := l.Task(pkg.Reference.String())
	timer := ui.LogElapsed(l, "exec")
	err := e.state.CacheAndUnpack(l.Task(pkg.Reference.String()), pkg)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, dep := range deps {
		if dep.Reference.Compare(pkg.Reference) == 0 {
			continue
		}
		err := e.state.CacheAndUnpack(l.Task(dep.Reference.String()), dep)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	pkg, err = e.Resolve(l, manifest.ExactSelector(pkg.Reference), true)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := e.EnsureChannelIsUpToDate(l, pkg); err != nil {
		return errors.WithStack(err)
	}
	runtimeDeps, err := e.ensureRuntimeDepsPresent(l, pkg)
	if err != nil {
		return errors.WithStack(err)
	}
	binaries, err := pkg.ResolveBinaries()
	if err != nil {
		return errors.WithStack(err)
	}

	installed, err := e.ListInstalled(l)
	if err != nil {
		return errors.WithStack(err)
	}
	ops := e.allEnvarOpsForPackages(runtimeDeps, installed...)
	packageHermitBin, err := e.getPackageRuntimeEnvops(pkg)
	if err != nil {
		return errors.WithStack(err)
	}
	if packageHermitBin != nil {
		ops = append(ops, packageHermitBin)
	}
	env := e.envarsFromOps(true, ops)

	if err != nil {
		return errors.WithStack(err)
	}
	for _, bin := range binaries {
		if filepath.Base(bin) != filepath.Base(binary) {
			continue
		}
		argsCopy := make([]string, len(args))
		copy(argsCopy, args)
		argsCopy[0] = bin
		b.Tracef("exec %s", shellquote.Join(argsCopy...))
		l.Clear()
		timer()

		err = syscall.Exec(bin, argsCopy, env)
		return errors.Wrapf(err, "%s: failed to execute %q", pkg, bin)
	}
	return errors.Errorf("%s: could not find binary %q", pkg, binary)
}

func (e *Env) getPackageRuntimeEnvops(pkg *manifest.Package) (envars.Op, error) {
	// If the package contains a Hermit env, add that to the PATH for runtime dependencies
	pkgEnv, err := OpenEnv(pkg.Root, e.state, e.packageSource, nil, e.httpClient, e.scriptSums)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if err = pkgEnv.Verify(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, errors.WithStack(err)
		}
		return nil, nil
	}
	return &envars.Prepend{Name: "PATH", Value: pkgEnv.binDir}, nil
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

// ValidationOptions for manifest validation
type ValidationOptions struct {
	// CheckSources if true, check that the package sources are reachable
	CheckSources bool
}

// ValidateManifest with given name.
//
// Returns the resolution errors for core systems as warnings.
// If a version fails to resolve for all systems, returns an error.
func (e *Env) ValidateManifest(l *ui.UI, name string, options *ValidationOptions) ([]string, error) {
	sources, err := e.sources(l)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	mnf, err := manifest.NewLoader(sources).Load(l, name)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	refs := mnf.References(name)
	var warnings []string
	task := l.Task("validate")
	for _, ref := range refs {
		task.Infof("Validating %s", ref)
		w, err := e.validateReference(l, sources, ref, options)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		warnings = append(warnings, w...)
	}

	return warnings, nil
}

func (e *Env) validateReference(l *ui.UI, srcs *sources.Sources, ref manifest.Reference, options *ValidationOptions) ([]string, error) {
	var fails []string
	var warnings []string
	for _, p := range platform.Core {
		resolver, err := manifest.New(srcs, manifest.Config{
			Env:   e.envDir,
			State: e.state.Root(),
			OS:    p.OS,
			Arch:  p.Arch,
		})
		if err != nil {
			return nil, errors.WithStack(err)
		}

		pkg, err := resolver.Resolve(l, manifest.ExactSelector(ref))
		if err != nil {
			msg := fmt.Sprintf("%s: %s", p, err.Error())
			fails = append(fails, msg)
			warnings = append(warnings, msg)
			continue
		}

		if options.CheckSources {
			if err := manifest.ValidatePackageSource(e.packageSource, e.httpClient, pkg.Source); err != nil {
				return nil, errors.Wrapf(err, "%s: %s", ref, p)
			}
		}

		warnings = append(warnings, pkg.Warnings...)
	}
	if len(fails) >= len(platform.Core) {
		return nil, errors.Errorf("%s failed to resolve on all platforms: %s", ref, strings.Join(fails, "; "))
	}
	return warnings, nil
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
	err := e.state.WritePackageState(pkg)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ListInstalledReferences from this environment.
//
// This function is much faster than ListInstalled, if all you need is the Reference.
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
	ops, err := e.EnvOps(l)
	if err != nil {
		return nil, err
	}
	return e.envarsFromOps(inherit, ops), nil
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
	return e.allEnvarOpsForPackages(nil, pkgs...), nil
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
	task := l.Task(pkg.Reference.String())
	if pkg.UpdateInterval == 0 || pkg.UpdatedAt.After(time.Now().Add(-1*pkg.UpdateInterval)) {
		task.Tracef("No updated required")
		// No updates needed for this package
		return nil
	}
	return errors.WithStack(e.state.UpgradeChannel(task, pkg))
}

// AddSource adds a new source bundle and refreshes the packages from it
func (e *Env) AddSource(l *ui.UI, s sources.Source) error {
	sources, err := e.sources(l)
	if err != nil {
		return errors.WithStack(err)
	}
	sources.Add(s)
	return e.Update(l, true)
}

// EnvDir returns the directory where this environment is rooted
func (e *Env) EnvDir() string {
	return e.envDir
}

// BinDir returns the directory for the binaries of this environment
func (e *Env) BinDir() string {
	return e.binDir
}

// upgradeVersion upgrades the package to its latest version.
//
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
	if err := resolved.EnsureSupported(); err != nil {
		return nil, errors.Wrapf(err, "upgrade failed")
	}
	if !resolved.Reference.Version.Match(pkg.Reference.Version) {
		l.Task(pkg.Reference.Name).SubTask("upgrade").Infof("Upgrading %s to %s", pkg, resolved)
		uc, err := e.uninstall(l.Task(pkg.Reference.String()), pkg)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		ic, err := e.install(l, resolved)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// Update the package.
		*pkg = *resolved
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
func (e *Env) allEnvarOpsForPackages(runtimeDeps []*manifest.Package, pkgs ...*manifest.Package) envars.Ops {
	var ops envars.Ops
	ops = append(ops, e.hermitEnvarOps()...)
	ops = append(ops, e.hermitRuntimeDepOps(runtimeDeps)...)
	ops = append(ops, e.envarsForPackages(pkgs...)...)
	ops = append(ops, e.localEnvarOps()...)
	ops = append(ops, e.ephemeralEnvars...)
	return ops
}

// Converts given envars.Ops into actual environment variables
// If "inherit" is true, system envars will be included.
func (e *Env) envarsFromOps(inherit bool, ops envars.Ops) []string {
	system := envars.Parse(os.Environ())
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

// hermitEnvarOps returns the environment variables created and required by hermit itself
func (e *Env) hermitEnvarOps() envars.Ops {
	return envars.Ops{
		&envars.Prepend{Name: "PATH", Value: e.binDir},
		&envars.Force{Name: "HERMIT_BIN", Value: e.binDir},
		&envars.Force{Name: "HERMIT_ENV", Value: e.envDir},
	}
}

// hermitRuntimeDepOps returns the environment variables for runtime dependencies
func (e *Env) hermitRuntimeDepOps(pkgs []*manifest.Package) envars.Ops {
	ops := e.envarsForPackages(pkgs...)
	for _, pkg := range pkgs {
		ops = append(ops, &envars.Prepend{Name: "PATH", Value: filepath.Join(e.state.BinaryDir(), pkg.Reference.String())})
	}
	return ops
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

// Update sources and auto-update channels.
//
// Will be updated at most every SyncFrequency unless "force" is true.
//
// A Sources set can only be synchronised once. Following calls will not have any effect.
func (e *Env) Update(l *ui.UI, force bool) error {
	resolver, err := e.resolver(l)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := resolver.Sync(l, force); err != nil {
		return errors.WithStack(err)
	}
	pkgs, err := e.ListInstalled(l)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, pkg := range pkgs {
		if pkg.Reference.IsChannel() {
			log := l.Task(pkg.String())
			if force || time.Since(pkg.UpdatedAt) > pkg.UpdateInterval {
				if err := e.state.UpgradeChannel(log, pkg); err != nil {
					return errors.Wrap(err, pkg.String())
				}
			} else {
				log.Debugf("Update skipped, updated within the last %s", pkg.UpdateInterval)
			}
		}
	}
	return nil
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
func (e *Env) ResolveWithDeps(l *ui.UI, installed []manifest.Reference, selector manifest.Selector, out map[string]*manifest.Package) (err error) {
	for _, existing := range installed {
		if existing.String() == selector.Name() {
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
			if err = e.ResolveWithDeps(l, installed, manifest.NameSelector(req), out); err != nil {
				return errors.WithStack(err)
			}
		} else if err != nil {
			return errors.WithStack(err)
		} else {
			err = e.ResolveWithDeps(l, installed, manifest.ExactSelector(ref), out)
			if err != nil {
				return errors.WithStack(err)
			}
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

// tidySha256Db is a helper function to remove leading and trailing
// whitespaces and empty lines from a multiline string containing
// SHA-256 hash digests, and return a slice of digest strings that do
// not start with a '#' character (used as a comment line indicator).
func tidySha256Db(s string) []string {
	ss := strings.Split(s, "\n")

	var res []string
	for _, x := range ss {
		line := strings.TrimSpace(x)
		// Add to result only when hash digest length is correct. Skip
		// comment line.
		if len(line) == 64 && !strings.HasPrefix(line, "#") {
			res = append(res, line)
		}
	}

	return res
}
