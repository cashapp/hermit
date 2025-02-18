package app

import (
	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
)

type initCmd struct {
	NoGit   bool     `help:"Disable Hermit's automatic management of Git'"`
	Idea    bool     `help:"Enable Hermit's automatic addition of its IntelliJ IDEA plugin"`
	Sources []string `help:"Sources to sync package manifests from."`
	Dir     string   `arg:"" help:"Directory to create environment in (${default})." default:"${env}" predictor:"dir"`
}

func (i *initCmd) Run(w *ui.UI, config Config, userConfig UserConfig) error {
	_, sum, err := GenInstaller(config)
	if err != nil {
		return errors.WithStack(err)
	}

	w.Tracef("Command line sources: %v", i.Sources)
	w.Tracef("User config: %+v", userConfig)

	sources := i.Sources
	if len(sources) == 0 {
		w.Tracef("Using init-sources from user config: %v", userConfig.InitSources)
		sources = userConfig.InitSources
	}

	return hermit.Init(w, i.Dir, config.BaseDistURL, hermit.UserStateDir, hermit.Config{
		Sources:     sources,
		ManageGit:   !i.NoGit,
		AddIJPlugin: i.Idea,
	}, sum)
}
