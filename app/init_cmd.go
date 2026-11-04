package app

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/redact"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type initCmd struct {
	Git     *bool        `negatable:"" help:"Enable Hermit's automatic management of Git'"`
	Idea    bool         `negatable:"" help:"Enable Hermit's automatic addition of its IntelliJ IDEA plugin"`
	Sources []redact.URL `help:"Sources to sync package manifests from."`
	Dir     string       `arg:"" help:"Directory to create environment in (${default})." default:"${env}" predictor:"dir"`

	ToolVersions bool `negatable:"" default:"true" help:"Install the tools pinned in an asdf or mise .tool-versions file in the environment directory, if present."`
}

func (i *initCmd) Run(w *ui.UI, config Config, userConfig UserConfig, cache *cache.Cache, sta *state.State, globalState GlobalState, defaultHTTPClient *http.Client) error {
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
	if i.Git != nil {
		hermitConfig.ManageGit = *i.Git
	}
	if i.Idea {
		hermitConfig.AddIJPlugin = true
	}

	if err := hermit.Init(w, i.Dir, config.BaseDistURL, hermit.UserStateDir, hermitConfig, sum); err != nil {
		return err
	}

	if !i.ToolVersions {
		return nil
	}
	toolVersions := filepath.Join(i.Dir, toolVersionsFile)
	if _, err := os.Stat(toolVersions); err != nil {
		return nil //nolint:nilerr // No .tool-versions file to import.
	}
	envInfo, err := hermit.LoadEnvInfo(i.Dir)
	if err != nil {
		return errors.WithStack(err)
	}
	env, err := hermit.OpenEnv(envInfo, sta, cache.GetSource, globalState.Env, defaultHTTPClient, config.SHA256Sums)
	if err != nil {
		return errors.WithStack(err)
	}
	return importToolVersions(w, env, sta, toolVersions)
}
