package app

type manifestCmd struct {
	Validate    validateSourceCmd `cmd:"" help:"Check a package manifest source for errors." group:"global"`
	AutoVersion autoVersionCmd    `cmd:"" help:"Upgrade manifest versions automatically where possible." group:"global"`
	Create      manifestCreateCmd `cmd:"" help:"Create a new manifest from an existing package artefact URL." group:"global"`
	LockDigests lockDigestsCmd    `cmd:"" help:"Lock the Digests for installed packages using sha256sums." group:"global"`
}
