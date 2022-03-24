package app

type unactivatedValidateCmd struct {
	Source validateSourceCmd `default:"withargs" cmd:"" help:"Check a package manifest source for errors." group:"global"`
	Env    validateEnvCmd    `cmd:"" help:"Verify an environment." group:"global"`
}

type activatedValidateCmd struct {
	unactivatedValidateCmd
	Script validateScriptCmd `cmd:"" help:"Verify a shell script uses only builtin or Hermit commands." group:"env"`
}
