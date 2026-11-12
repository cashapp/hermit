package app

import (
	"time"

	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type gcCmd struct {
	Age time.Duration `help:"Remove cached state not used within this duration." default:"168h"`
}

func (g *gcCmd) Run(l *ui.UI, sta *state.State) error {
	return sta.GC(l, g.Age)
}
