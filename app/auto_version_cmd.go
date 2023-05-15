package app

import (
	"io/ioutil"
	"net/http"
	"os"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/manifest/autoversion"
	"github.com/cashapp/hermit/manifest/digest"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type autoVersionCmd struct {
	ContinueOnError bool     `help:"Continue on errors."`
	UpdateDigests   bool     `help:"Update digests when auto-versioning."`
	Manifest        []string `arg:"" type:"existingfile" required:"" help:"Manifests to upgrade." predictor:"hclfile"`
}

func (a *autoVersionCmd) Run(l *ui.UI, hclient *http.Client, state *state.State, client *github.Client) error {
	for _, path := range a.Manifest {
		l.Debugf("Auto-versioning %s", path)
		info, err := os.Stat(path)
		if err != nil {
			return errors.WithStack(err)
		}
		original, err := ioutil.ReadFile(path)
		if err != nil {
			return errors.WithStack(err)
		}
		err = autoVersionManifest(l, hclient, state, client, path)
		if err != nil {
			if !a.ContinueOnError {
				return errors.Wrap(err, path)
			}
			l.Warnf("Could not update digests for %q: %s", path, err)
			err = ioutil.WriteFile(path, original, info.Mode())
			if err != nil {
				return errors.Wrapf(err, "could not restore original manifest: %s", path)
			}
		}
	}
	return nil
}

func autoVersionManifest(l *ui.UI, hclient *http.Client, state *state.State, client *github.Client, path string) error {
	version, err := autoversion.AutoVersion(hclient, client, path)
	if err != nil {
		return errors.WithStack(err)
	}
	if version == "" {
		return nil
	}
	l.Infof("Auto-versioned %s to %s", path, version)
	err = digest.UpdateDigests(l, hclient, state, path)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
