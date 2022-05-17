package app

import (
	"encoding/json"
	"fmt"
	"go/doc"
	"os"
	"sort"
	"strings"

	"github.com/alecthomas/colour"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/ui"
)

// JSONFormattable contains the shared JSON boolean flag for Kong
type JSONFormattable struct {
	JSON bool `help:"Format information as a JSON array" default:"false"`
}

type listCmd struct {
	Short bool `short:"s" help:"Short listing."`
	JSONFormattable
}

func (cmd *listCmd) Run(l *ui.UI, env *hermit.Env) error {
	pkgs, err := env.ListInstalled(l)
	if err != nil {
		return errors.WithStack(err)
	}
	if cmd.Short {
		for _, pkg := range pkgs {
			fmt.Println(pkg)
		}
		return nil
	}
	err = listPackages(pkgs, false, cmd.JSON, l)
	if err != nil {
		return errors.Wrapf(err, "error listing packages")
	}

	return nil
}

func listPackages(pkgs manifest.Packages, allVersions bool, isJSON bool, l *ui.UI) error {
	byName := map[string][]*manifest.Package{}
	for _, pkg := range pkgs {
		name := pkg.Reference.Name
		byName[name] = append(byName[name], pkg)
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	w, _, _ := terminal.GetSize(0)
	if w == -1 {
		w = 80
	}

	packages := make([]*manifest.Package, 0)

	for _, name := range names {
		pkgs := byName[name]

		if isJSON {
			packages = append(packages, pkgs...)
			continue
		}

		var versions []string
		for _, pkg := range pkgs {
			if !allVersions && !pkg.Linked {
				continue
			}
			clr := ""
			suffix := ""
			if pkg.Unsupported() {
				clr = "^1"
				suffix = " (architecture not supported)"
			} else if pkg.Linked {
				switch pkg.State {
				case manifest.PackageStateRemote:
					clr = "^1"
				case manifest.PackageStateDownloaded:
					clr = "^3"
				case manifest.PackageStateInstalled:
					clr = "^2"
				}
			}
			versions = append(versions, fmt.Sprintf("%s%s%s^R", clr, pkg.Reference.StringNoName(), suffix))
		}
		colour.Println("^B^2" + name + "^R (" + strings.Join(versions, ", ") + ")")
		doc.ToText(os.Stdout, pkgs[0].Description, "  ", "", w-2)
	}

	if isJSON {
		content, err := json.Marshal(packages)
		if err != nil {
			return errors.Wrapf(err, "error formatting packages output to json")
		}

		l.Printf("%s\n", string(content))

	}

	return nil
}
