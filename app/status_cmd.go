package app

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/ui"
)

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
