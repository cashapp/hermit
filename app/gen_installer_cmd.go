package app

import (
	"bytes"
	"crypto/sha256"

	// Embed installer template
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/alecthomas/hcl"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/state"
)

var (
	//go:embed "install.sh.tmpl"
	installerTemplateSource string
	installerTemplate       = template.Must(template.New("install.sh").Funcs(template.FuncMap{
		"string": func(b []byte) string { return string(b) },
		"words":  func(s []string) string { return strings.Join(s, " ") },
	}).Parse(installerTemplateSource))
)

type genInstallerCmd struct {
	Schema bool   `help:"Display the global schema."`
	Dest   string `required:"" placeholder:"FILE" help:"Where to write the installer script."`
}

type params struct {
	DistURL      string
	InstallPaths []string
}

// GenInstaller generates an instaler script from the app configuration.
// It returns a byte slice of the generated installer script, its
// SHA-256 digest as a hexadecimal string, and any error encountered.
func GenInstaller(config Config) ([]byte, string, error) {
	var b bytes.Buffer
	p := params{
		DistURL:      config.BaseDistURL,
		InstallPaths: config.InstallPaths,
	}
	err := installerTemplate.Execute(&b, p)
	if err != nil {
		return nil, "", errors.WithStack(err)
	}
	sha256sum := sha256.Sum256(b.Bytes())
	return b.Bytes(), hex.EncodeToString(sha256sum[:]), nil
}

func (g *genInstallerCmd) Run(config Config) error {
	if g.Schema {
		ast, err := hcl.Schema(&state.Config{})
		if err != nil {
			return errors.WithStack(err)
		}
		schema, err := hcl.MarshalAST(ast)
		if err != nil {
			return errors.WithStack(err)
		}
		fmt.Printf("%s\n", schema)
		return nil
	}

	w, err := os.Create(g.Dest)
	if err != nil {
		return errors.WithStack(err)
	}
	defer w.Close() // nolint
	script, sum, err := GenInstaller(config)
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
