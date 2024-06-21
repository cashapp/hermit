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
	envInfo, err := hermit.LoadEnvInfo(v.Env)
	if err != nil {
		return errors.WithStack(err)
	}
	env, err := hermit.OpenEnv(envInfo, state, cache.GetSource, nil, httpClient, config.SHA256Sums)
	if err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(env.Verify())
}
