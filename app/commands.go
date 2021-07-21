package app

import (
	"encoding/json"
	"fmt"
	"go/doc"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/colour"
	"github.com/alecthomas/kong"
	"github.com/pkg/errors"
	sshterminal "golang.org/x/crypto/ssh/terminal"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util/debug"
)

// GlobalState configurable by user to be passed through to Hermit.
type GlobalState struct {
	Env         envars.Envars `help:"Extra environment variables to apply to environments."`
	ShortPrompt bool          `help:"Use a minimal prompt in active environments."`
}

type cliCommon interface {
	getCPUProfile() string
	getMemProfile() string
	getDebug() bool
	getTrace() bool
	getQuiet() bool
	getLevel() ui.Level
	getGlobalState() GlobalState
}

// CLI structure.
type unactivated struct {
	VersionFlag kong.VersionFlag `help:"Show version." name:"version"`
	CPUProfile  string           `placeholder:"PATH" name:"cpu-profile" help:"Enable CPU profiling to PATH." hidden:""`
	MemProfile  string           `placeholder:"PATH" name:"mem-profile" help:"Enable memory profiling to PATH." hidden:""`
	Debug       bool             `help:"Enable debug logging." short:"d"`
	Trace       bool             `help:"Enable trace logging." short:"t"`
	Quiet       bool             `help:"Disable logging and progress UI, except fatal errors." env:"HERMIT_QUIET" short:"q"`
	Level       ui.Level         `help:"Set minimum log level." env:"HERMIT_LOG" default:"info" enum:"trace,debug,info,warn,error,fatal"`
	GlobalState

	Init       initCmd       `cmd:"" help:"Initialise an environment (idempotent)." group:"env"`
	Version    versionCmd    `cmd:"" help:"Show version." group:"global"`
	Validate   validateCmd   `hidden:"" cmd:"" help:"Check a package manifest source for errors." group:"global"`
	Manifest   manifestCmd   `cmd:"" help:"Commands for manipulating manifests."`
	Info       infoCmd       `cmd:"" help:"Show information on packages." group:"global"`
	ShellHooks shellHooksCmd `cmd:"" help:"Manage Hermit auto-activation hooks of a shell." group:"global" aliases:"install-hooks"`

	Noop                 noopCmd              `cmd:"" help:"No-op, just exit." hidden:""`
	Activate             activateCmd          `cmd:"" help:"Activate an environment." hidden:""`
	Exec                 execCmd              `cmd:"" help:"Directly execute a binary in a package." hidden:""`
	Sync                 syncCmd              `cmd:"" help:"Sync manifest sources." group:"global"`
	Search               searchCmd            `cmd:"" help:"Search for packages to install." group:"global"`
	DumpDB               dumpDBCmd            `cmd:"" help:"Dump state database." hidden:""`
	DumpUserConfigSchema dumpUserConfigSchema `cmd:"" help:"Dump user configuration schema." hidden:""`

	kong.Plugins
}

func (u *unactivated) getCPUProfile() string       { return u.CPUProfile }
func (u *unactivated) getMemProfile() string       { return u.MemProfile }
func (u *unactivated) getTrace() bool              { return u.Trace }
func (u *unactivated) getDebug() bool              { return u.Debug }
func (u *unactivated) getQuiet() bool              { return u.Quiet }
func (u *unactivated) getLevel() ui.Level          { return u.Level }
func (u *unactivated) getGlobalState() GlobalState { return u.GlobalState }

type activated struct {
	unactivated

	Status    statusCmd    `cmd:"" help:"Show status of Hermit environment." group:"env"`
	Install   installCmd   `cmd:"" help:"Install packages." group:"env"`
	Uninstall uninstallCmd `cmd:"" help:"Uninstall packages." group:"env"`
	Upgrade   upgradeCmd   `cmd:"" help:"Upgrade packages" group:"env"`
	List      listCmd      `cmd:"" help:"List local packages." group:"env"`
	Exec      execCmd      `cmd:"" help:"Directly execute a binary in a package." group:"env"`
	Env       envCmd       `cmd:"" help:"Manage environment variables." group:"env"`

	Clean cleanCmd `cmd:"" help:"Clean hermit cache." group:"global"`
	GC    gcCmd    `cmd:"" help:"Garbage collect unused Hermit packages and clean the download cache." group:"global"`
	Test  testCmd  `cmd:"" help:"Run package sanity tests." group:"global"`

	// TODO: Remove this after we can assume that all active hermit sessions have been recreated with the latest scripts
	Deactivate deactivateCmd `cmd:"" help:"Deprecated" hidden:""`
}

type versionCmd struct{}

func (v *versionCmd) Run(kctx kong.Vars) error {
	fmt.Println(kctx["version"])
	return nil
}

type initCmd struct {
	NoGit   bool     `help:"Disable Hermit's automatic management of Git'"`
	Sources []string `help:"Sources to sync package manifests from."`
	Dir     string   `arg:"" help:"Directory to create environment in (${default})." default:"${env}" predictor:"dir"`
}

func (i *initCmd) Run(w *ui.UI, config Config) error {
	return hermit.Init(w, i.Dir, config.BaseDistURL, hermit.UserStateDir, hermit.Config{
		Sources:   i.Sources,
		ManageGit: !i.NoGit,
	})
}

type infoCmd struct {
	Packages []string `arg:"" required:"" help:"Packages to retrieve information for" predictor:"package"`
	JSON     bool     `help:"Format information as a JSON array" default:"false"`
}

func (i *infoCmd) Run(l *ui.UI, env *hermit.Env, sta *state.State) error {
	var installed map[string]*manifest.Package
	packages := []*manifest.Package{}
	for _, name := range i.Packages {
		selector, err := manifest.GlobSelector(name)
		if err != nil {
			return errors.WithStack(err)
		}
		var pkg *manifest.Package
		if env != nil {
			if installed == nil {
				installed, err = getInstalledPackageMap(l, env)
				if err != nil {
					return errors.WithStack(err)
				}
			}
			// If the selector is an exact package name match with an installed package we'll just use it.
			if pkg = installed[selector.String()]; pkg == nil {
				pkg, err = env.Resolve(l, selector, false)
				if err != nil {
					return errors.WithStack(err)
				}
			}

		} else {
			pkg, err = sta.Resolve(l, selector)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		packages = append(packages, pkg)
	}

	envroot := "<env>" // Used as a place holder in env vars if there is no active environment
	if env != nil {
		envroot = env.Root()
	}

	if i.JSON {
		js, err := json.Marshal(packages)
		if err != nil {
			return errors.WithStack(err)
		}
		l.Printf("%s\n", string(js))
		return nil
	}

	for j, pkg := range packages {
		colour.Printf("^B^2Name:^R %s\n", pkg.Reference.Name)
		if pkg.Reference.Version.IsSet() {
			colour.Printf("^B^2Version:^R %s\n", pkg.Reference.Version)
		} else {
			colour.Printf("^B^2Channel:^R %s\n", pkg.Reference.Channel)
		}
		colour.Printf("^B^2Description:^R %s\n", pkg.Description)
		colour.Printf("^B^2State:^R %s\n", pkg.State)
		if !pkg.LastUsed.IsZero() {
			colour.Printf("^B^2Last used:^R %s ago\n", time.Since(pkg.LastUsed))
		}
		colour.Printf("^B^2Source:^R %s\n", pkg.Source)
		colour.Printf("^B^2Root:^R %s\n", pkg.Root)
		if len(pkg.Requires) != 0 {
			colour.Printf("^B^2Requires:^R %s\n", strings.Join(pkg.Requires, " "))
		}
		if len(pkg.Provides) != 0 {
			colour.Printf("^B^2Provides:^R %s\n", strings.Join(pkg.Provides, " "))
		}
		environ := envars.Parse(os.Environ()).Apply(envroot, pkg.Env).Changed(false)
		if len(environ) != 0 {
			colour.Printf("^B^2Envars:^R\n")
			for key, value := range environ {
				colour.Printf("  %s=%s\n", key, shell.Quote(value))
			}
		}
		bins, _ := pkg.ResolveBinaries()
		for i := range bins {
			bins[i] = filepath.Base(bins[i])
		}
		if len(bins) > 0 {
			colour.Printf("^B^2Binaries:^R %s\n", strings.Join(bins, " "))
		}
		if j < len(i.Packages)-1 {
			colour.Printf("\n")
		}
	}
	return nil
}

func getInstalledPackageMap(l *ui.UI, env *hermit.Env) (map[string]*manifest.Package, error) {
	installedPkgs, err := env.ListInstalled(l)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	installed := make(map[string]*manifest.Package, len(installedPkgs))
	for _, installedPkg := range installedPkgs {
		installed[installedPkg.Reference.Name] = installedPkg
	}
	return installed, nil
}

type noopCmd struct{}

func (n *noopCmd) Run() error { return nil }

type activateCmd struct {
	Dir string `arg:"" help:"Directory of environment to activate (${default})" default:"${env}"`
}

func (a *activateCmd) Run(l *ui.UI, sta *state.State, globalState GlobalState) error {
	realdir, err := resolveActivationDir(a.Dir)
	if err != nil {
		return errors.WithStack(err)
	}
	env, err := hermit.OpenEnv(realdir, sta, globalState.Env)
	if err != nil {
		return errors.WithStack(err)
	}
	messages, err := env.Trigger(l, manifest.EventEnvActivate)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, message := range messages {
		fmt.Fprintln(os.Stderr, message)
	}
	ops, err := env.EnvOps(l)
	if err != nil {
		return errors.WithStack(err)
	}
	pkgs, err := env.ListInstalled(l)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, pkg := range pkgs {
		pkg.LogWarnings(l)
	}
	sh, err := shell.Detect()
	if err != nil {
		return errors.WithStack(err)
	}
	environ := envars.Parse(os.Environ()).Apply(env.Root(), ops).Changed(true)
	return shell.ActivateHermit(os.Stdout, sh, shell.ActivationConfig{
		Env:         environ,
		Root:        env.Root(),
		ShortPrompt: globalState.ShortPrompt,
	})
}

// resolveActivationDir converts the directory used at activation to an absolute path
// with all symlinks resolved
func resolveActivationDir(from string) (string, error) {
	result, err := filepath.Abs(from)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return filepath.EvalSymlinks(result)
}

type deactivateCmd struct{}

func (a *deactivateCmd) Run(env *hermit.Env, p *ui.UI) error {
	ops, err := env.EnvOps(p)
	if err != nil {
		return errors.WithStack(err)
	}
	sh, err := shell.Detect()
	if err != nil {
		return errors.WithStack(err)
	}
	environ := envars.Parse(os.Environ()).Revert(env.Root(), ops).Changed(true)
	return shell.DeactivateHermit(os.Stdout, sh, environ)
}

type statusCmd struct{}

func (s *statusCmd) Run(l *ui.UI, env *hermit.Env) error {
	envars, err := env.Envars(l, false)
	if err != nil {
		return errors.WithStack(err)
	}
	pkgs, err := env.ListInstalled(l)
	if err != nil {
		return errors.WithStack(err)
	}
	fmt.Println("Sources:")
	sources, err := env.Sources(l)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, source := range sources {
		fmt.Printf("  %s\n", source)
	}
	fmt.Println("Packages:")
	for _, pkg := range pkgs {
		fmt.Printf("  %s\n", pkg)
		if l.WillLog(ui.LevelDebug) {
			fmt.Printf("    Description: %s\n", pkg.Description)
			fmt.Printf("    Root: %s\n", pkg.Root)
			fmt.Printf("    Source: %s\n", pkg.Source)
			bins, err := pkg.ResolveBinaries()
			if err != nil {
				return errors.WithStack(err)
			}
			fmt.Println("    Binaries:")
			for _, bin := range bins {
				fmt.Printf("      %s\n", bin)
			}
			if len(pkg.Env) != 0 {
				fmt.Printf("    Environment:\n")
				for _, op := range pkg.Env {
					fmt.Printf("      %s\n", op)
				}
			}
		}
	}
	fmt.Println("Environment:")
	for _, env := range envars {
		fmt.Printf("  %s\n", env)
	}
	return nil
}

type syncCmd struct{}

func (s *syncCmd) Run(l *ui.UI, env *hermit.Env, state *state.State) error {
	self, err := os.Executable()
	if err != nil {
		return errors.WithStack(err)
	}
	srcs, err := state.Sources(l)
	if err != nil {
		return errors.WithStack(err)
	}
	// Sync sources from either the env or default sources.
	if env != nil {
		err = env.Sync(l, true)
	} else {
		err = srcs.Sync(l, true)
	}
	if err != nil {
		return errors.WithStack(err)
	}
	// Upgrade hermit if necessary
	pkgRef := filepath.Base(filepath.Dir(self))
	if strings.HasPrefix(pkgRef, "hermit@") {
		pkg, err := state.Resolve(l, manifest.ExactSelector(manifest.ParseReference(pkgRef)))
		if err != nil {
			return errors.WithStack(err)
		}
		err = state.UpgradeChannel(l.Task(pkgRef), pkg)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

type testCmd struct {
	Pkg []string `arg:"" required:"" help:"Run sanity tests for these packages."`
}

func (t *testCmd) Run(l *ui.UI, env *hermit.Env) error {
	for _, name := range t.Pkg {
		selector, err := manifest.GlobSelector(name)
		if err != nil {
			return errors.WithStack(err)
		}
		pkg, err := env.Resolve(l, selector, false)
		if err != nil {
			return errors.WithStack(err)
		}
		if err = env.Test(l, pkg); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

type cleanCmd struct {
	Bin       bool `short:"b" help:"Clean links out of the local bin directory."`
	Packages  bool `short:"p" help:"Clean all extracted packages."`
	Cache     bool `short:"c" help:"Clean download cache."`
	Transient bool `short:"a" help:"Clean everything transient (packages, cache)."`
}

func (c *cleanCmd) Run(l *ui.UI, env *hermit.Env) error {
	var mask hermit.CleanMask
	if c.Bin {
		mask |= hermit.CleanBin
	}
	if c.Packages {
		mask |= hermit.CleanPackages
	}
	if c.Cache {
		mask |= hermit.CleanCache
	}
	if c.Transient {
		mask = hermit.CleanTransient
	}
	if mask == 0 {
		return errors.New("no targets to clean, try --help")
	}
	return env.Clean(l, mask)
}

type execCmd struct {
	Binary string   `arg:"" help:"Binary symlink to execute."`
	Args   []string `arg:"" help:"Arguments to pass to executable (use -- to separate)." optional:""`
}

func (e *execCmd) Run(l *ui.UI, sta *state.State, env *hermit.Env, globalState GlobalState) error {
	envDir, err := hermit.EnvDirFromProxyLink(e.Binary)
	if err != nil {
		return errors.WithStack(err)
	}
	if env == nil {
		env, err = hermit.OpenEnv(envDir, sta, globalState.Env)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	args := []string{e.Binary}
	args = append(args, e.Args...)

	self, err := os.Executable()
	if err != nil {
		return errors.WithStack(err)
	}

	// Upgrade hermit if necessary
	pkgRef := filepath.Base(filepath.Dir(self))
	if strings.HasPrefix(pkgRef, "hermit@") {
		err := updateHermit(l, env, pkgRef, false)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// Special-case executing Hermit itself.
	if filepath.Base(e.Binary) == "hermit" {
		env := os.Environ()
		env = append(env, "HERMIT_ENV="+envDir)
		return syscall.Exec(self, args, env)
	}

	// Check that if we are running from an activated environment, it is the correct one
	activeEnv := os.Getenv("ACTIVE_HERMIT")
	if activeEnv != "" && envDir != "" && activeEnv != envDir {
		return errors.New("can not execute a Hermit managed binary from a non active environment")
	}

	pkg, binary, err := env.ResolveLink(l, e.Binary)
	if err != nil {
		return errors.WithStack(err)
	}
	installed, err := env.ListInstalled(l)
	if err != nil {
		return errors.WithStack(err)
	}

	// Collect dependencies we might have to install
	// if they are not in the cache
	deps := map[string]*manifest.Package{}
	err = env.ResolveWithDeps(l, installed, manifest.ExactSelector(pkg.Reference), deps)
	if err != nil {
		return errors.WithStack(err)
	}

	return env.Exec(l, pkg, binary, args, deps)
}

func updateHermit(l *ui.UI, env *hermit.Env, pkgRef string, force bool) error {
	l.Tracef("Checking if %s needs to be updated", pkgRef)
	pkg, err := env.Resolve(l, manifest.ExactSelector(manifest.ParseReference(pkgRef)), false)
	if err != nil {
		return errors.WithStack(err)
	}
	// Mark Hermit updated if this is a new installation to prevent immediate upgrade checks
	if force {
		pkg.UpdatedAt = time.Time{}
	} else if pkg.UpdatedAt.IsZero() {
		pkg.UpdatedAt = time.Now()
	}
	err = env.UpdateUsage(pkg)
	if err != nil {
		return errors.WithStack(err)
	}

	if debug.Flags.AlwaysCheckSelf {
		// set the update time to 0 to force an update check
		pkg.UpdatedAt = time.Time{}
	}
	return errors.WithStack(env.EnsureChannelIsUpToDate(l, pkg))
}

type envCmd struct {
	Raw        bool   `short:"r" help:"Output raw values without shell quoting."`
	Activate   bool   `xor:"envars" help:"Prints the commands needed to set the environment to the activated state"`
	Deactivate bool   `xor:"envars" help:"Prints the commands needed to reset the environment to the deactivated state"`
	Inherit    bool   `short:"i" help:"Inherit variables from parent environment."`
	Names      bool   `short:"n" help:"Show only names."`
	Unset      bool   `short:"u" help:"Unset the specified environment variable."`
	Name       string `arg:"" optional:"" help:"Name of the environment variable."`
	Value      string `arg:"" optional:"" help:"Value to set the variable to."`
}

func (e *envCmd) Help() string {
	return `
Without arguments the "env" command will display environment variables for the active Hermit environment.

Passing "<name>" will print the value for that environment variable.

Passing "<name> <value>" will set the value for an environment variable in the active Hermit environment."
	`
}

func (e *envCmd) Run(l *ui.UI, env *hermit.Env) error {
	// Special case for backwards compatibility.
	// TODO: Remove this at some point.
	if e.Name == "get" {
		e.Name = e.Value
		e.Value = ""
	}

	// Setting envar
	if e.Value != "" {
		return env.SetEnv(e.Name, e.Value)
	}

	if e.Unset {
		return env.DelEnv(e.Name)
	}

	if e.Activate {
		sh, err := shell.Detect()
		if err != nil {
			return errors.WithStack(err)
		}
		ops, err := env.EnvOps(l)
		if err != nil {
			return errors.WithStack(err)
		}
		environ := envars.Parse(os.Environ()).Apply(env.Root(), ops).Changed(true)
		return errors.WithStack(sh.ApplyEnvars(os.Stdout, environ))
	}

	if e.Deactivate {
		sh, err := shell.Detect()
		if err != nil {
			return errors.WithStack(err)
		}
		ops, err := env.EnvOps(l)
		if err != nil {
			return errors.WithStack(err)
		}
		environ := envars.Parse(os.Environ()).Revert(env.Root(), ops).Changed(true)
		return errors.WithStack(sh.ApplyEnvars(os.Stdout, environ))
	}

	// Display envars.
	envars, err := env.Envars(l, e.Inherit)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, v := range envars {
		parts := strings.SplitN(v, "=", 2)
		name := parts[0]
		value := parts[1]
		if e.Name != "" {
			if name == e.Name {
				fmt.Println(value)
				break
			}
			continue
		}
		if e.Names {
			fmt.Println(name)
		} else {
			if !e.Raw {
				value = shell.Quote(value)
			}
			fmt.Printf("%s=%s\n", name, value)
		}
	}
	return nil
}

type searchCmd struct {
	Short      bool   `short:"s" help:"Short listing."`
	Constraint string `arg:"" help:"Package regex." optional:""`
}

func (s *searchCmd) Run(l *ui.UI, env *hermit.Env, state *state.State) error {
	var (
		pkgs manifest.Packages
		err  error
	)
	if env != nil {
		err = env.Sync(l, false)
		if err != nil {
			return errors.WithStack(err)
		}
		pkgs, err = env.Search(l, s.Constraint)
		if err != nil {
			return errors.WithStack(err)
		}
	} else {
		srcs, err := state.Sources(l)
		if err != nil {
			return errors.WithStack(err)
		}
		err = srcs.Sync(l, false)
		if err != nil {
			return errors.WithStack(err)
		}
		pkgs, err = state.Search(l, s.Constraint)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	if s.Short {
		for _, pkg := range pkgs {
			fmt.Println(pkg)
		}
		return nil
	}
	listPackages(pkgs, true)
	return nil
}

type listCmd struct {
	Short bool `short:"s" help:"Short listing."`
}

func (cmd *listCmd) Run(l *ui.UI, env *hermit.Env) error {
	pkgs, err := env.ListInstalled(l)
	if err != nil {
		return errors.WithStack(err)
	}
	if cmd.Short {
		for _, pkg := range pkgs {
			fmt.Println(pkg)
		}
		return nil
	}
	listPackages(pkgs, false)
	return nil
}

type uninstallCmd struct {
	Packages []string `arg:"" help:"Packages to uninstall from this environment." predictor:"installed-package"`
}

func (u *uninstallCmd) Run(l *ui.UI, env *hermit.Env) error {
	installed, err := env.ListInstalled(l)
	if err != nil {
		return errors.WithStack(err)
	}
	selectors := []manifest.Selector{}
	for _, pkg := range u.Packages {
		globs, err := manifest.GlobSelector(pkg)
		if err != nil {
			return errors.WithStack(err)
		}
		selectors = append(selectors, globs)
	}
	changes := shell.NewChanges(envars.Parse(os.Environ()))
next:
	for _, selector := range selectors {
		for _, pkg := range installed {
			if selector.Matches(pkg.Reference) {
				c, err := env.Uninstall(l, pkg)
				if err != nil {
					return errors.WithStack(err)
				}
				changes = changes.Merge(c)
				continue next
			}
		}
		return errors.Errorf("package %s is not installed", selector)
	}

	return nil
}

type installCmd struct {
	Packages []string `arg:"" optional:"" name:"package" help:"Packages to install (<name>[-<version>]). Version can be a glob to find the latest version with." predictor:"package"`
}

func (i *installCmd) Help() string {
	return `
Add the specified set of packages to the environment. If no packages are specified, all existing packages linked
into the environment will be downloaded and installed. Packages will be pinned to the version resolved at install time.
`
}

func (i *installCmd) Run(l *ui.UI, env *hermit.Env, state *state.State) error {
	installed, err := env.ListInstalled(l)
	if err != nil {
		return errors.WithStack(err)
	}
	pkgs := map[string]*manifest.Package{}
	packages := i.Packages

	err = env.Sync(l, false)
	if err != nil {
		return errors.WithStack(err)
	}

	if len(packages) == 0 {
		// Checking that all the packages are downloaded and unarchived
		for _, pkg := range installed {
			task := l.Task(pkg.Reference.String())
			err := state.CacheAndUnpack(task, pkg)
			pkg.LogWarnings(l)
			task.Done()
			if err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}
	selectors := make([]manifest.Selector, len(packages))
	// Check that we are not installing an already existing package
	for i, search := range packages {
		selector, err := manifest.GlobSelector(search)
		if err != nil {
			return errors.WithStack(err)
		}
		selectors[i] = selector
		for _, ipkg := range installed {
			if selector.Matches(ipkg.Reference) {
				return errors.Errorf("%s cannot be installed as %s is already installed", selector.String(), ipkg.Reference)
			}
		}
	}
	for i, search := range packages {
		err := env.ResolveWithDeps(l, installed, selectors[i], pkgs)
		if err != nil {
			return errors.Wrap(err, search)
		}
	}
	changes := shell.NewChanges(envars.Parse(os.Environ()))
	w := l.WriterAt(ui.LevelInfo)
	defer w.Sync() // nolint
	for _, pkg := range pkgs {
		// Skip possible dependencies that have already been installed
		exists := false
		for _, ipkg := range installed {
			if ipkg.Reference.String() == pkg.Reference.String() {
				exists = true
				break
			}
		}
		if exists {
			continue
		}

		c, err := env.Install(l, pkg)
		if err != nil {
			return errors.WithStack(err)
		}
		messages, err := env.Trigger(l, manifest.EventInstall)
		if err != nil {
			return errors.WithStack(err)
		}
		for _, message := range messages {
			fmt.Fprintln(w, message)
		}
		changes = changes.Merge(c)
		pkg.LogWarnings(l)
	}
	return nil
}

type gcCmd struct {
	Age time.Duration `help:"Age of packages to garbage collect." default:"168h"`
}

func (g *gcCmd) Run(l *ui.UI, env *hermit.Env) error {
	return env.GC(l, g.Age)
}

type upgradeCmd struct {
	Packages []string `arg:"" optional:"" name:"package" help:"Packages to upgrade. If omitted, upgrades all installed packages."  predictor:"installed-package"`
}

func (g *upgradeCmd) Run(l *ui.UI, env *hermit.Env) error {
	packages := []*manifest.Package{}
	installed, err := env.ListInstalled(l)
	if err != nil {
		return errors.WithStack(err)
	}
	if g.Packages != nil {
		// check that the requested packages have been installed
		packageNames := map[string]*manifest.Package{}
		for _, pkg := range installed {
			packageNames[pkg.Reference.Name] = pkg
		}
		for _, name := range g.Packages {
			if packageNames[name] == nil {
				return errors.Errorf("no installed package '%s' found.", name)
			}
			packages = append(packages, packageNames[name])
		}
	} else {
		packages = installed
	}

	changes := shell.NewChanges(envars.Parse(os.Environ()))

	// upgrade packages
	for _, pkg := range packages {
		c, err := env.Upgrade(l, pkg)
		if err != nil {
			return errors.WithStack(err)
		}
		changes = changes.Merge(c)
	}

	return nil
}

type validateCmd struct {
	Source string `arg:"" optional:"" name:"source" help:"The manifest source to validate."`
}

func (g *validateCmd) Run(l *ui.UI, env *hermit.Env, sta *state.State) error {
	var (
		srcs    *sources.Sources
		err     error
		merrors manifest.ManifestErrors
	)
	if env != nil && g.Source == "" {
		merrors, err = env.ValidateManifests(l)
		if err != nil {
			return errors.WithStack(err)
		}
	} else {
		srcs, err = sources.ForURIs(l, sta.SourcesDir(), "", []string{g.Source})
		if err != nil {
			return errors.WithStack(err)
		}
		resolver, err := manifest.New(srcs, manifest.Config{
			State: sta.Root(),
			OS:    runtime.GOOS,
			Arch:  runtime.GOARCH,
		})
		if err != nil {
			return errors.WithStack(err)
		}
		err = resolver.LoadAll()
		if err != nil {
			return errors.WithStack(err)
		}
	}

	if len(merrors) > 0 {
		merrors.LogErrors(l)
		return errors.New("the source had " + strconv.Itoa(len(merrors)) + " broken manifest files")
	}

	l.Infof("No errors found")
	return nil
}

func listPackages(pkgs manifest.Packages, allVersions bool) {
	byName := map[string][]*manifest.Package{}
	for _, pkg := range pkgs {
		name := pkg.Reference.Name
		byName[name] = append(byName[name], pkg)
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	w, _, _ := sshterminal.GetSize(0)
	if w == -1 {
		w = 80
	}
	for _, name := range names {
		pkgs := byName[name]
		var versions []string
		for _, pkg := range pkgs {
			if !allVersions && !pkg.Linked {
				continue
			}
			clr := ""
			if pkg.Linked {
				switch pkg.State {
				case manifest.PackageStateRemote:
					clr = "^1"
				case manifest.PackageStateDownloaded:
					clr = "^3"
				case manifest.PackageStateInstalled:
					clr = "^2"
				}
			}
			versions = append(versions, fmt.Sprintf("%s%s^R", clr, pkg.Reference.StringNoName()))
		}
		colour.Println("^B^2" + name + "^R (" + strings.Join(versions, ", ") + ")")
		doc.ToText(os.Stdout, pkgs[0].Description, "  ", "", w-2)
	}
}

type shellHooksCmd struct {
	Zsh   bool `xor:"shell" help:"Update Zsh hooks."`
	Bash  bool `xor:"shell" help:"Update Bash hooks."`
	Print bool `help:"Prints out the hook configuration code" hidden:"" `
}

func (s *shellHooksCmd) Run(l *ui.UI) error {
	var (
		sh  shell.Shell
		err error
	)
	if s.Bash {
		sh = &shell.Bash{}
	} else if s.Zsh {
		sh = &shell.Zsh{}
	} else {
		sh, err = shell.Detect()
		if err != nil {
			return errors.WithStack(err)
		}
	}
	if !s.Print {
		return errors.WithStack(shell.InstallHooks(l, sh))
	}
	return errors.WithStack(shell.PrintHooks(sh))
}

type dumpDBCmd struct{}

func (dumpDBCmd) Run(state *state.State) error {
	return state.DumpDB(os.Stdout)
}

type autoVersionCmd struct {
	Manifest []string `arg:"" type:"existingfile" required:"" help:"Manifests to upgrade."`
}

func (s *autoVersionCmd) Run(l *ui.UI) error {
	for _, path := range s.Manifest {
		l.Debugf("Auto-versioning %s", path)
		version, err := manifest.AutoVersion(path)
		if err != nil {
			return errors.WithStack(err)
		}
		if version != "" {
			l.Infof("Auto-versioned %s to %s", path, version)
		}
	}
	return nil
}

type manifestCmd struct {
	Validate    validateCmd    `cmd:"" help:"Check a package manifest source for errors." group:"global"`
	AutoVersion autoVersionCmd `cmd:"" help:"Upgrade manifest versions automatically where possible." group:"global"`
}

type dumpUserConfigSchema struct{}

func (dumpUserConfigSchema) Run() error {
	fmt.Print(userConfigSchema)
	return nil
}
