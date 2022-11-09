package app

import (
	"net/http"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type validateEnvCmd struct {
	Env string `arg:"" type:"existingdir" help:"Path to environment root."`
}

func (v *validateEnvCmd) Run(l *ui.UI, state *state.State, cache *cache.Cache, config Config, httpClient *http.Client) error {
	env, err := hermit.OpenEnv(v.Env, state, cache.GetSource, nil, httpClient, config.SHA256Sums, config.RequireDigests)
	if err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(env.Verify())
}
