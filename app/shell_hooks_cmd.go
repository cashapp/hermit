package app

import (
	"github.com/pkg/errors"

	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/ui"
)

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
