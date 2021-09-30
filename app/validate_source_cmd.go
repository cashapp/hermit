package app

import (
	"runtime"
	"strconv"

	"github.com/pkg/errors"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type validateSourceCmd struct {
	Source string `arg:"" optional:"" name:"source" help:"The manifest source to validate."`
}

func (g *validateSourceCmd) Run(l *ui.UI, env *hermit.Env, sta *state.State) error {
	var (
		srcs    *sources.Sources
		err     error
		merrors manifest.ManifestErrors
	)
	if env != nil && g.Source == "" {
		merrors, err = env.ValidateManifests(l)
		if err != nil {
			return errors.WithStack(err)
		}
	} else {
		srcs, err = sources.ForURIs(l, sta.SourcesDir(), "", []string{g.Source})
		if err != nil {
			return errors.WithStack(err)
		}
		resolver, err := manifest.New(srcs, manifest.Config{
			State: sta.Root(),
			OS:    runtime.GOOS,
			Arch:  runtime.GOARCH,
		})
		if err != nil {
			return errors.WithStack(err)
		}
		err = resolver.LoadAll()
		if err != nil {
			return errors.WithStack(err)
		}
	}

	if len(merrors) > 0 {
		merrors.LogErrors(l)
		return errors.New("the source had " + strconv.Itoa(len(merrors)) + " broken manifest files")
	}

	l.Infof("No errors found")
	return nil
}
