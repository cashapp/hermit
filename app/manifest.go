package app

import (
	"fmt"
	"net/http"

	"github.com/alecthomas/hcl"
	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/ui"
	"github.com/pkg/errors"
)

type manifestCmd struct {
	Validate    validateSourceCmd `cmd:"" help:"Check a package manifest source for errors." group:"global"`
	AutoVersion autoVersionCmd    `cmd:"" help:"Upgrade manifest versions automatically where possible." group:"global"`
	Create      manifestCreateCmd `cmd:"" help:"Create a new manifest from an existing package artefact URL." group:"global"`
}

type manifestCreateCmd struct {
	PkgVersion string `help:"Explicit version if required."`
	URL        string `arg:"" required:"" help:"URL of a package artefact."`
}

func (m *manifestCreateCmd) Run(p *ui.UI, defaultHTTPClient *http.Client, ghClient *github.Client) error {
	pkg, err := manifest.InferFromArtefact(p, defaultHTTPClient, ghClient, m.URL, m.PkgVersion)
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
