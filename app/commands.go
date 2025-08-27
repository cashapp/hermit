package app

import (
	"time"

	"github.com/alecthomas/kong"

	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/ui"
)

// GlobalState configurable by user to be passed through to Hermit.
type GlobalState struct {
	Env envars.Envars `help:"Extra environment variables to apply to environments."`
}

type cliInterface interface {
	getCPUProfile() string
	getMemProfile() string
	getDebug() bool
	getTrace() bool
	getQuiet() bool
	getLevel() ui.Level
	getGlobalState() GlobalState
	getLockTimeout() time.Duration
	getUserConfigFile() string
}

type cliBase struct {
	VersionFlag    kong.VersionFlag `help:"Show version." name:"version"`
	CPUProfile     string           `placeholder:"PATH" name:"cpu-profile" help:"Enable CPU profiling to PATH." hidden:""`
	MemProfile     string           `placeholder:"PATH" name:"mem-profile" help:"Enable memory profiling to PATH." hidden:""`
	Debug          bool             `help:"Enable debug logging." short:"d"`
	Trace          bool             `help:"Enable trace logging." short:"t"`
	Quiet          bool             `help:"Disable logging and progress UI, except fatal errors." env:"HERMIT_QUIET" short:"q"`
	Level          ui.Level         `help:"Set minimum log level (${enum})." env:"HERMIT_LOG" default:"auto" enum:"auto,trace,debug,info,warn,error,fatal"`
	LockTimeout    time.Duration    `help:"Timeout for waiting on the lock" default:"30s" env:"HERMIT_LOCK_TIMEOUT"`
	UserConfigFile string           `help:"Path to Hermit user configuration file." name:"user-config" default:"~/.hermit.hcl" env:"HERMIT_USER_CONFIG"`
	GlobalState

	Init       initCmd       `cmd:"" help:"Initialise an environment (idempotent)." group:"env"`
	Version    versionCmd    `cmd:"" help:"Show version." group:"global"`
	Manifest   manifestCmd   `cmd:"" help:"Commands for manipulating manifests."`
	Info       infoCmd       `cmd:"" help:"Show information on packages." group:"global"`
	ShellHooks shellHooksCmd `cmd:"" help:"Manage Hermit auto-activation hooks of a shell." group:"global" aliases:"install-hooks"`

	Noop                 noopCmd              `cmd:"" help:"No-op, just exit." hidden:"" passthrough:""`
	Activate             activateCmd          `cmd:"" help:"Activate an environment." hidden:""`
	Exec                 execCmd              `cmd:"" help:"Directly execute a binary in a package." hidden:""`
	Update               updateCmd            `cmd:"" aliases:"sync" help:"Update manifest sources." group:"global"`
	Search               searchCmd            `cmd:"" help:"Search for packages to install." group:"global"`
	DumpUserConfigSchema dumpUserConfigSchema `cmd:"" help:"Dump user configuration schema." hidden:""`
	ScriptSHA            scriptSHACmd         `cmd:"" help:"Print known sha256 sums of activate-hermit and hermit scripts." hidden:""`
	GenInstaller         genInstallerCmd      `cmd:"" help:"Generate Hermit installer script." group:"global"`
	kong.Plugins
}

var _ cliInterface = &cliBase{}

func (u *cliBase) getCPUProfile() string         { return u.CPUProfile }
func (u *cliBase) getMemProfile() string         { return u.MemProfile }
func (u *cliBase) getTrace() bool                { return u.Trace }
func (u *cliBase) getDebug() bool                { return u.Debug }
func (u *cliBase) getQuiet() bool                { return u.Quiet }
func (u *cliBase) getLevel() ui.Level            { return ui.AutoLevel(u.Level) }
func (u *cliBase) getGlobalState() GlobalState   { return u.GlobalState }
func (u *cliBase) getLockTimeout() time.Duration { return u.LockTimeout }
func (u *cliBase) getUserConfigFile() string     { return u.UserConfigFile }

// CLI structure.
type unactivated struct {
	cliBase
	Validate unactivatedValidateCmd `cmd:"" help:"Hermit validation." group:"global"`
}

type activated struct {
	cliBase

	Status     statusCmd            `cmd:"" help:"Show status of Hermit environment." group:"env"`
	Install    installCmd           `cmd:"" help:"Install packages." group:"env"`
	Uninstall  uninstallCmd         `cmd:"" help:"Uninstall packages." group:"env"`
	Upgrade    upgradeCmd           `cmd:"" help:"Upgrade packages" group:"env"`
	List       listCmd              `cmd:"" help:"List local packages." group:"env"`
	Env        envCmd               `cmd:"" help:"Manage environment variables." group:"env"`
	Validate   activatedValidateCmd `cmd:"" help:"Hermit validation." group:"global"`
	AddDigests addDigestsCmd        `cmd:"" help:"Add digests for all versions/platforms to the input manifest files." group:"global"`
	Bundle     bundleCmd            `cmd:"" help:"Expand packages from the current environment into a target directory." group:"env"`

	Clean cleanCmd `cmd:"" help:"Clean hermit cache." group:"global"`
	GC    gcCmd    `cmd:"" hidden:"" group:"global"`
	Test  testCmd  `cmd:"" help:"Run package sanity tests." group:"global"`

	// TODO: Remove this after we can assume that all active hermit sessions have been recreated with the latest scripts
	Deactivate deactivateCmd `cmd:"" help:"Deprecated" hidden:""`
}
