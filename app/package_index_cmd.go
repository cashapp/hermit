package app

import (
	"encoding/json"
	"os"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
	"github.com/qdm12/reprint"
)

type packageIndexCmd struct {
	Source []string `help:"Set of sources to index."`
}

func (i *packageIndexCmd) Run(config Config, l *ui.UI, cache *cache.Cache) error {
	var stateConfig state.Config
	if err := reprint.FromTo(&config.State, &stateConfig); err != nil {
		return errors.Wrap(err, "failed to copy config?!")
	}
	if len(i.Source) > 0 {
		stateConfig.Sources = i.Source
	}
	st, err := state.Open(hermit.UserStateDir, stateConfig, cache)
	if err != nil {
		return errors.WithStack(err)
	}
	pkgs, err := st.Search(l, "")
	if err != nil {
		return errors.WithStack(err)
	}
	err = json.NewEncoder(os.Stdout).Encode(pkgs)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
