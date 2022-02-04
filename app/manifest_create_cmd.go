package app

import (
	"fmt"
	"net/http"

	"github.com/alecthomas/hcl"

	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/ui"
)

type manifestCreateCmd struct {
	PkgVersion string `help:"Explicit version if required."`
	URL        string `arg:"" required:"" help:"URL of a package artefact."`
}

func (m *manifestCreateCmd) Run(p *ui.UI, cache *cache.Cache, defaultHTTPClient *http.Client, ghClient *github.Client) error {
	pkg, err := manifest.InferFromArtefact(p, cache.GetSource, defaultHTTPClient, ghClient, m.URL, m.PkgVersion)
	if err != nil {
		return errors.WithStack(err)
	}
	data, err := hcl.Marshal(pkg)
	if err != nil {
		return errors.WithStack(err)
	}
	fmt.Printf("%s\n", data)
	return nil
}
