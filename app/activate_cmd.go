package app

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/shell"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type activateCmd struct {
	Dir         string `arg:"" help:"Directory of environment to activate (${default})" default:"${env}"`
	Prompt      string `enum:"default,env,short,none" default:"default" help:"Include hermit environment, just icon or nothing in shell prompt"`
	ShortPrompt bool   `help:"Use a minimal prompt in active environments." hidden:""`
}

func (a *activateCmd) Run(l *ui.UI, cache *cache.Cache, sta *state.State, globalState GlobalState, config Config, defaultClient *http.Client, userConfig UserConfig) error {
	realdir, err := resolveActivationDir(a.Dir)
	if err != nil {
		return errors.WithStack(err)
	}
	envInfo, err := hermit.LoadEnvInfo(realdir)
	if err != nil {
		return errors.WithStack(err)
	}
	env, err := hermit.OpenEnv(envInfo, sta, cache.GetSource, globalState.Env, defaultClient, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	messages, err := env.Trigger(l, manifest.EventEnvActivate)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, message := range messages {
		fmt.Fprintln(os.Stderr, message)
	}
	ops, err := env.EnvOps(l)
	if err != nil {
		return errors.WithStack(err)
	}
	pkgs, err := env.ListInstalled(l)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, pkg := range pkgs {
		pkg.LogWarnings(l)
	}
	sh, err := shell.Detect()
	if err != nil {
		return errors.WithStack(err)
	}
	environ := envars.Parse(os.Environ()).Apply(env.Root(), ops).Changed(true)
	// Apply user config settings
	prompt := userConfig.Prompt
	if userConfig.ShortPrompt {
		prompt = "short"
	}
	// Apply command line overrides
	if a.Prompt != "default" {
		prompt = a.Prompt
	}
	if a.ShortPrompt {
		prompt = "short"
	}
	return shell.ActivateHermit(os.Stdout, sh, shell.ActivationConfig{
		Env:    environ,
		Root:   env.Root(),
		Prompt: prompt,
	})
}

// resolveActivationDir converts the directory used at activation to an absolute path
// with all symlinks resolved
func resolveActivationDir(from string) (string, error) {
	result, err := filepath.Abs(from)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return filepath.EvalSymlinks(result)
}
