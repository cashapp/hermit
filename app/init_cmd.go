package app

import (
	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/ui"
)

type initCmd struct {
	NoGit   bool     `help:"Disable Hermit's automatic management of Git'"`
	Sources []string `help:"Sources to sync package manifests from."`
	Dir     string   `arg:"" help:"Directory to create environment in (${default})." default:"${env}" predictor:"dir"`
}

func (i *initCmd) Run(w *ui.UI, config Config) error {
	return hermit.Init(w, i.Dir, config.BaseDistURL, hermit.UserStateDir, hermit.Config{
		Sources:   i.Sources,
		ManageGit: !i.NoGit,
	})
}
