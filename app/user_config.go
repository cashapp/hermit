package app

import (
	"io/ioutil"
	"os"

	"github.com/alecthomas/hcl"
	"github.com/alecthomas/kong"
	"github.com/pkg/errors"
)

const userConfigPath = "~/.hermit.hcl"

var userConfigSchema = func() string {
	schema, err := hcl.Schema(&UserConfig{})
	if err != nil {
		return ""
	}
	data, err := hcl.MarshalAST(schema)
	if err != nil {
		return ""
	}
	return string(data)
}()

// UserConfig is stored in ~/.hermit.hcl
type UserConfig struct {
	Prompt      string `hcl:"prompt,optional" default:"env" enum:"env,short,none" help:"Modify prompt to include hermit environment (env), just an icon (short) or nothing (none)"`
	ShortPrompt bool   `hcl:"short-prompt,optional" help:"If true use a short prompt when an environment is activated."`
	NoGit       bool   `hcl:"no-git,optional" help:"If true Hermit will never add/remove files from Git automatically."`
}

// LoadUserConfig from disk.
func LoadUserConfig() (UserConfig, error) {
	config := UserConfig{}
	// always return a valid config on error, with defaults set.
	_ = hcl.Unmarshal([]byte{}, &config)
	data, err := ioutil.ReadFile(kong.ExpandPath(userConfigPath))
	if os.IsNotExist(err) {
		return config, nil
	} else if err != nil {
		return config, errors.WithStack(err)
	}
	err = hcl.Unmarshal(data, &config)
	if err != nil {
		return config, errors.WithStack(err)
	}
	return config, nil
}

// UserConfigResolver is a Kong configuration resolver for the Hermit user configuration file.
func UserConfigResolver(userConfig UserConfig) kong.Resolver {
	return &userConfigResolver{userConfig}
}

type userConfigResolver struct{ config UserConfig }

func (u *userConfigResolver) Validate(app *kong.Application) error { return nil }
func (u *userConfigResolver) Resolve(context *kong.Context, parent *kong.Path, flag *kong.Flag) (interface{}, error) {
	switch flag.Name {
	case "no-git":
		return u.config.NoGit, nil

	case "prompt":
		return u.config.Prompt, nil

	case "short-prompt":
		return u.config.ShortPrompt, nil

	default:
		return nil, nil
	}
}
