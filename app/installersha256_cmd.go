package app

import (
	"fmt"

	"github.com/alecthomas/kong"
)

type installersha256Cmd struct{}

func (v *installersha256Cmd) Run(kctx kong.Vars) error {
	fmt.Println(kctx["installersha256"])
	return nil
}
