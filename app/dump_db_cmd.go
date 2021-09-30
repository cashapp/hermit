package app

import (
	"os"

	"github.com/cashapp/hermit/state"
)

type dumpDBCmd struct{}

func (dumpDBCmd) Run(state *state.State) error {
	return state.DumpDB(os.Stdout)
}
