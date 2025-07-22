package app

import (
	"os"

	"github.com/alecthomas/hcl"
	"github.com/alecthomas/kong"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/errors"
)

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
	Prompt      string        `hcl:"prompt,optional" default:"env" enum:"env,short,none" help:"Modify prompt to include hermit environment (env), just an icon (short) or nothing (none)"`
	ShortPrompt bool          `hcl:"short-prompt,optional" help:"If true use a short prompt when an environment is activated."`
	NoGit       bool          `hcl:"no-git,optional" help:"If true Hermit will never add/remove files from Git automatically."`
	Idea        bool          `hcl:"idea,optional" help:"If true Hermit will try to add the IntelliJ IDEA plugin automatically."`
	Defaults    hermit.Config `hcl:"defaults,block,optional" help:"Default configuration values for new Hermit environments."`
}

func NewUserConfigWithDefaults() UserConfig {
	return UserConfig{
		Defaults: hermit.Config{
			ManageGit: true,
		},
	}
}

// IsUserConfigExists checks if the user config file exists at the given path.
func IsUserConfigExists(configPath string) bool {
	_, err := os.Stat(kong.ExpandPath(configPath))
	return err == nil
}

// LoadUserConfig from disk.
func LoadUserConfig(configPath string) (UserConfig, error) {
	config := NewUserConfigWithDefaults()
	// always return a valid config on error, with defaults set.
	_ = hcl.Unmarshal([]byte{}, &config)
	data, err := os.ReadFile(kong.ExpandPath(configPath))
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
