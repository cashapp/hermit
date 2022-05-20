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

func buildListJSONResult(byName map[string][]*manifest.Package, names []string) interface{} {
	packages := make([]*manifest.Package, 0)

	for _, name := range names {
		pg := byName[name]

		for _, pkg := range pg {
			if pkg.Linked {
				packages = append(packages, pkg)
			}
		}
	}

	return packages
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
	err = listPackages(pkgs, &listPackageOption{
		AllVersions:   false,
		TransformJSON: buildListJSONResult,
		UI:            l,
		JSON:          cmd.JSON,
	})
	if err != nil {
		return errors.Wrapf(err, "error listing packages")
	}

	return nil
}

func groupPackages(pkgs []*manifest.Package) (map[string][]*manifest.Package, []string) {
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

	return byName, names
}

// transformPackagesToJSON transforms the given grouped packages and ordered names into the output JSON struct
type transformPackagesToJSON func(byName map[string][]*manifest.Package, names []string) interface{}

func listPackagesInJSONFormat(pkgs manifest.Packages, option *listPackageOption) error {
	byName, names := groupPackages(pkgs)

	val := option.TransformJSON(byName, names)
	content, err := json.Marshal(val)
	if err != nil {
		return errors.Wrapf(err, "error formatting packages output to json")
	}

	option.UI.Printf("%s\n", string(content))

	return nil
}

type listPackageOption struct {
	AllVersions   bool
	TransformJSON transformPackagesToJSON
	UI            *ui.UI
	JSON          bool
	Prefix        string
}

// NameList is a list of package names plus the prefix it searched for
type NameList struct {
	nl     []string
	prefix string
}

func (n NameList) Len() int {
	return len(n.nl)
}
func (n NameList) Swap(i, j int) {
	n.nl[i], n.nl[j] = n.nl[j], n.nl[i]
}

// implements prefix-first search over this list
func (n NameList) Less(i, j int) bool {
	if n.prefix != "" {
		left := strings.HasPrefix(n.nl[i], n.prefix)
		right := strings.HasPrefix(n.nl[j], n.prefix)
		if left && right {
			return n.nl[i] < n.nl[j]
		} else if left {
			return true
		} else if right {
			return false
		}
	}
	return n.nl[i] < n.nl[j]
}

func listPackagesInCLI(pkgs manifest.Packages, option *listPackageOption) {
	byName, names := groupPackages(pkgs)
	nl := NameList{names, option.Prefix}
	sort.Sort(nl)

	w, _, _ := terminal.GetSize(0)
	if w == -1 {
		w = 80
	}

	for _, name := range nl.nl {
		printPackage(byName[name], option, name, w)
	}
}

func printPackage(pkgs []*manifest.Package, option *listPackageOption, name string, w int) {
	var versions []string
	for _, pkg := range pkgs {
		if !option.AllVersions && !pkg.Linked {
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

func listPackages(pkgs manifest.Packages, option *listPackageOption) error {
	if option.JSON {
		return errors.WithStack(listPackagesInJSONFormat(pkgs, option))
	}

	listPackagesInCLI(pkgs, option)

	return nil
}
