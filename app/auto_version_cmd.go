package app

import (
	"net/http"

	"github.com/cashapp/hermit/state"

	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/manifest/autoversion"
	"github.com/cashapp/hermit/ui"
)

type autoVersionCmd struct {
	Manifest []string `arg:"" type:"existingfile" required:"" help:"Manifests to upgrade." predictor:"hclfile"`
}

func (s *autoVersionCmd) Run(l *ui.UI, hclient *http.Client, client *github.Client, state *state.State) error {
	for _, path := range s.Manifest {
		l.Debugf("Auto-versioning %s", path)
		version, err := autoversion.AutoVersion(hclient, client, path, state, l)
		if err != nil {
			l.Warnf("could not auto-version %q: %s", path, err)
			continue
		}
		if version != "" {
			l.Infof("Auto-versioned %s to %s", path, version)
		}
	}
	return nil
}
