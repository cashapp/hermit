package app

import (
	"time"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/ui"
)

type gcCmd struct {
	Age time.Duration `help:"Age of packages to garbage collect." default:"168h"`
}

func (g *gcCmd) Run(l *ui.UI, env *hermit.Env) error {
	return env.GC(l, g.Age)
}
