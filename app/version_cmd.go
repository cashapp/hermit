package app

import (
	"fmt"

	"github.com/alecthomas/kong"
)

type versionCmd struct{}

func (v *versionCmd) Run(kctx kong.Vars) error {
	fmt.Println(kctx["version"])
	return nil
}
