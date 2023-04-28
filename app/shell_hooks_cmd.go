package app

import (
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/ui"
)

type shellHooksCmd struct {
	Zsh   bool `xor:"shell" help:"Update Zsh hooks."`
	Bash  bool `xor:"shell" help:"Update Bash hooks."`
	Print bool `help:"Prints out the hook configuration code" hidden:"" `
}

func (s *shellHooksCmd) Run(cli cliInterface, l *ui.UI, config Config) error {
	var (
		sh  shell.Shell
		err error

		bin = cli.getHermitBin()
	)

	if s.Bash {
		sh = &shell.Bash{Bin: bin}
	} else if s.Zsh {
		sh = &shell.Zsh{Bin: bin}
	} else {
		sh, err = shell.Detect(bin)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	if !s.Print {
		return errors.WithStack(shell.InstallHooks(l, sh))
	}

	return errors.WithStack(shell.PrintHooks(sh, config.SHA256Sums))
}
