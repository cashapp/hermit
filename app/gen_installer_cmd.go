package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/errors"
)

var (
	installerTemplateSource = hermit.InstallerTemplateSource
	installerTemplate       = template.Must(template.New("install.sh").Funcs(template.FuncMap{
		"string": func(b []byte) string { return string(b) },
		"words":  func(s []string) string { return strings.Join(s, " ") },
	}).Parse(installerTemplateSource))
)

type genInstallerCmd struct {
	Dest         string   `required:"" placeholder:"FILE" help:"Where to write the installer script."`
	InstallPaths []string `placeholder:"PATH" help:"Possible system-wide installation paths." default:"$${HOME}/bin,/opt/homebrew/bin,/usr/local/bin"`
}

type params struct {
	DistURL      string
	InstallPaths []string
}

// genInstaller generates an instaler script from the app configuration and the
// system install paths passed in as a CLI parameter.
// It returns a byte slice of the generated installer script, its
// SHA-256 digest as a hexadecimal string, and any error encountered.
func (g *genInstallerCmd) genInstaller(config Config) ([]byte, string, error) {
	var b bytes.Buffer
	p := params{
		DistURL:      config.BaseDistURL,
		InstallPaths: g.InstallPaths,
	}
	err := installerTemplate.Execute(&b, p)
	if err != nil {
		return nil, "", errors.WithStack(err)
	}
	sha256sum := sha256.Sum256(b.Bytes())
	return b.Bytes(), hex.EncodeToString(sha256sum[:]), nil
}

func (g *genInstallerCmd) Run(config Config) error {
	w, err := os.Create(g.Dest)
	if err != nil {
		return errors.WithStack(err)
	}
	defer w.Close() // nolint
	script, sum, err := g.genInstaller(config)
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = w.Write(script)
	if err != nil {
		return errors.WithStack(err)
	}
	fmt.Println(sum)
	return nil
}
