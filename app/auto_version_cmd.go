package app

import (
	"net/http"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/manifest/autoversion"
	"github.com/cashapp/hermit/manifest/digest"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type autoVersionCmd struct {
	UpdateDigests bool     `help:"Update digests when auto-versioning."`
	Manifest      []string `arg:"" type:"existingfile" required:"" help:"Manifests to upgrade." predictor:"hclfile"`
}

func (a *autoVersionCmd) Run(l *ui.UI, hclient *http.Client, state *state.State, client *github.Client) error {
	for _, path := range a.Manifest {
		l.Debugf("Auto-versioning %s", path)
		version, err := autoversion.AutoVersion(hclient, client, path)
		if err != nil {
			l.Warnf("could not auto-version %q: %s", path, err)
			continue
		}
		if version != "" && a.UpdateDigests {
			l.Infof("Auto-versioned %s to %s", path, version)
			if err := digest.UpdateDigests(l, hclient, state, path); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	return nil
}
