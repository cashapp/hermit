package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"github.com/cashapp/hermit/app"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

var (
	baseDistURL = "https://github.com/cashapp/hermit/releases/download/"
	channel     = "canary"
	version     = "devel"
	//go:embed builtin
	builtin embed.FS
)

func main() {
	level, err := ui.LevelFromString(envOrDefault("HERMIT_LOG", "info"))
	if err != nil {
		level = ui.LevelInfo
	}

	builtin, err := fs.Sub(builtin, "builtin")
	if err != nil {
		panic("this should never happen")
	}

	app.Main(app.Config{
		LogLevel:    level,
		BaseDistURL: baseDistURL + channel,
		Version:     fmt.Sprintf("%s (%s)", version, channel),
		State: state.Config{
			Builtin: sources.NewBuiltInSource(builtin),
		},
		CI: os.Getenv("CI") != "",
	})
}

func envOrDefault(name, dflt string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return dflt
}
