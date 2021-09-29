package app

import (
	"fmt"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"github.com/pkg/errors"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

// GlobalState configurable by user to be passed through to Hermit.
type GlobalState struct {
	Env envars.Envars `help:"Extra environment variables to apply to environments."`
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
	GlobalState

	VersionFlag kong.VersionFlag `help:"Show version." name:"version"`
	CPUProfile  string           `placeholder:"PATH" name:"cpu-profile" help:"Enable CPU profiling to PATH." hidden:""`
	MemProfile  string           `placeholder:"PATH" name:"mem-profile" help:"Enable memory profiling to PATH." hidden:""`
	Debug       bool             `help:"Enable debug logging." short:"d"`
	Trace       bool             `help:"Enable trace logging." short:"t"`
	Quiet       bool             `help:"Disable logging and progress UI, except fatal errors." env:"HERMIT_QUIET" short:"q"`
	Level       ui.Level         `help:"Set minimum log level." env:"HERMIT_LOG" default:"info" enum:"trace,debug,info,warn,error,fatal"`

	Init     initCmd    `cmd:"" help:"Initialise an environment (idempotent)." group:"env"`
	Version  versionCmd `cmd:"" help:"Show version." group:"global"`
	Validate struct {
		Source validateSourceCmd `default:"withargs" cmd:"" help:"Check a package manifest source for errors." group:"global"`
		Env    validateEnvCmd    `cmd:"" help:"Validate an environment." group:"global"`
	} `cmd:"" help:"Hermit validation." group:"global"`
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

type noopCmd struct{}

func (n *noopCmd) Run() error { return nil }

type gcCmd struct {
	Age time.Duration `help:"Age of packages to garbage collect." default:"168h"`
}

func (g *gcCmd) Run(l *ui.UI, env *hermit.Env) error {
	return env.GC(l, g.Age)
}

type shellHooksCmd struct {
	Zsh   bool `xor:"shell" help:"Update Zsh hooks."`
	Bash  bool `xor:"shell" help:"Update Bash hooks."`
	Print bool `help:"Prints out the hook configuration code" hidden:"" `
}

func (s *shellHooksCmd) Run(l *ui.UI, config Config) error {
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
	return errors.WithStack(shell.PrintHooks(sh, config.SHA256Sums))
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

type dumpUserConfigSchema struct{}

func (dumpUserConfigSchema) Run() error {
	fmt.Print(userConfigSchema)
	return nil
}
