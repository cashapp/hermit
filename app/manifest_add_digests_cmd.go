package app

import (
	"net/http"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest/digest"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type addDigestsCmd struct {
	Manifest []string `arg:"" help:"List of files that need to be updated with digests"`
}

func (*addDigestsCmd) Help() string {
	return `
	This command will go through each manifest file in input and add missing digest values to the "sha256" map.

	Note: It might download packages that are not in the local cache. So it might take some time.
	`
}

func (a *addDigestsCmd) Run(l *ui.UI, client *http.Client, state *state.State) error {
	for _, f := range a.Manifest {
		err := digest.UpdateDigests(l, client, state, f)
		if err != nil {
			return errors.Wrap(err, f)
		}
	}
	return nil
}
