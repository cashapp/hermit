package app

import (
	"github.com/alecthomas/kong"

	"github.com/cashapp/hermit/envars"
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
	VersionFlag kong.VersionFlag `help:"Show version." name:"version"`
	CPUProfile  string           `placeholder:"PATH" name:"cpu-profile" help:"Enable CPU profiling to PATH." hidden:""`
	MemProfile  string           `placeholder:"PATH" name:"mem-profile" help:"Enable memory profiling to PATH." hidden:""`
	Debug       bool             `help:"Enable debug logging." short:"d"`
	Trace       bool             `help:"Enable trace logging." short:"t"`
	Quiet       bool             `help:"Disable logging and progress UI, except fatal errors." env:"HERMIT_QUIET" short:"q"`
	Level       ui.Level         `help:"Set minimum log level (${enum})." env:"HERMIT_LOG" default:"auto" enum:"auto,trace,debug,info,warn,error,fatal"`
	GlobalState

	Init       initCmd       `cmd:"" help:"Initialise an environment (idempotent)." group:"env"`
	Version    versionCmd    `cmd:"" help:"Show version." group:"global"`
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
	Validate             validateCmd          `cmd:"" help:"Hermit validation." group:"global"`

	kong.Plugins
}

func (u *unactivated) getCPUProfile() string       { return u.CPUProfile }
func (u *unactivated) getMemProfile() string       { return u.MemProfile }
func (u *unactivated) getTrace() bool              { return u.Trace }
func (u *unactivated) getDebug() bool              { return u.Debug }
func (u *unactivated) getQuiet() bool              { return u.Quiet }
func (u *unactivated) getLevel() ui.Level          { return ui.AutoLevel(u.Level) }
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
