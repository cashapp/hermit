package main

import (
	_ "embed" // Embedding.
	"fmt"
	"os"
	"text/template"

	"github.com/alecthomas/hcl"
	"github.com/alecthomas/kong"
	"github.com/cashapp/hermit/state"
)

var (
	//go:embed "install.sh.tmpl"
	installerTemplateSource string
	installerTemplate       = template.Must(template.New("install.sh").Funcs(template.FuncMap{
		"string": func(b []byte) string { return string(b) },
	}).Parse(installerTemplateSource))
)

var cli struct {
	Schema  bool   `help:"Display the global schema."`
	Dest    string `required:"" placeholder:"FILE" help:"Where to write the installer script."`
	DistURL string `required:"" placeholder:"URL" help:"Base distribution URL."`
}

func main() {
	kctx := kong.Parse(&cli)
	if cli.Schema {
		ast, err := hcl.Schema(&state.Config{})
		kctx.FatalIfErrorf(err)
		schema, err := hcl.MarshalAST(ast)
		kctx.FatalIfErrorf(err)
		fmt.Printf("%s\n", schema)
		kctx.Exit(0)
	}
	w, err := os.Create(cli.Dest)
	kctx.FatalIfErrorf(err)
	defer w.Close() // nolint
	err = installerTemplate.Execute(w, cli)
	kctx.FatalIfErrorf(err)
}
