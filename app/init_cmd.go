package app

import (
	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
)

type initCmd struct {
	Git     bool     `negatable:"" help:"Enable Hermit's automatic management of Git'"`
	Idea    bool     `negatable:"" help:"Enable Hermit's automatic addition of its IntelliJ IDEA plugin"`
	Sources []string `help:"Sources to sync package manifests from."`
	Dir     string   `arg:"" help:"Directory to create environment in (${default})." default:"${env}" predictor:"dir"`
}

func (i *initCmd) Run(w *ui.UI, config Config, userConfig UserConfig) error {
	_, sum, err := GenInstaller(config)
	if err != nil {
		return errors.WithStack(err)
	}

	// Load defaults from user config (or zero value)
	hermitConfig := userConfig.Defaults

	// Apply top-level user config settings
	if userConfig.NoGit {
		hermitConfig.ManageGit = false
	}
	if userConfig.Idea {
		hermitConfig.AddIJPlugin = true
	}

	// Apply command line overrides (these take precedence over everything)
	if i.Sources != nil {
		hermitConfig.Sources = i.Sources
	}
	if !i.Git {
		hermitConfig.ManageGit = false
	}
	if i.Idea {
		hermitConfig.AddIJPlugin = true
	}

	return hermit.Init(w, i.Dir, config.BaseDistURL, hermit.UserStateDir, hermitConfig, sum)
}
