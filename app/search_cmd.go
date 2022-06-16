package app

import (
	"fmt"
	"regexp"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type searchCmd struct {
	Short   bool   `short:"s" help:"Short listing."`
	Pattern string `arg:"" help:"Either a search term or regex to match a package name." optional:""`
	Exact   bool   `short:"e" long:"exact" help:"Exact name matches only. Not compatible with regex patterns."`
	JSONFormattable
}

// searchResult resolved from a manifest.
type searchResult struct {
	Name           string
	Versions       []string
	Channels       []string
	CurrentVersion string
	Description    string
	Repository     string
}

// buildSearchResult constructs a search result from packages with same name
// p is an array expected to be package with same name
func buildSearchResult(p []*manifest.Package) *searchResult {
	out := &searchResult{
		Versions: make([]string, 0),
		Channels: make([]string, 0),
	}

	for _, pkg := range p {
		if out.Repository == "" {
			out.Repository = pkg.Repository
		}
		if out.Name == "" {
			out.Name = pkg.Reference.Name
			out.Description = pkg.Description
		}
		ver := pkg.Reference.StringNoName()
		if pkg.Reference.IsChannel() {
			out.Channels = append(out.Channels, ver)
		} else {
			out.Versions = append(out.Versions, ver)
		}
		if pkg.Linked {
			out.CurrentVersion = ver
		}
	}

	return out
}

func buildSearchJSONResults(byName map[string][]*manifest.Package, names []string) interface{} {
	packages := make([]*searchResult, 0)

	for _, name := range names {
		pg := byName[name]

		packages = append(packages, buildSearchResult(pg))
	}

	return packages
}

func (s *searchCmd) Run(l *ui.UI, env *hermit.Env, state *state.State) error {
	var (
		pkgs manifest.Packages
		err  error
	)
	pattern := s.Pattern
	if s.Exact {
		pattern = "^" + regexp.QuoteMeta(pattern) + "$"
	}
	if env != nil {
		err = env.Update(l, false)
		if err != nil {
			return errors.WithStack(err)
		}
		pkgs, err = env.Search(l, pattern)
		if err != nil {
			return errors.WithStack(err)
		}
	} else {
		srcs, err := state.Sources(l)
		if err != nil {
			return errors.WithStack(err)
		}
		err = srcs.Sync(l, false)
		if err != nil {
			return errors.WithStack(err)
		}
		pkgs, err = state.Search(l, pattern)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	if s.Short {
		for _, pkg := range pkgs {
			fmt.Println(pkg)
		}
		return nil
	}

	err = listPackages(pkgs, &listPackageOption{
		AllVersions:   true,
		TransformJSON: buildSearchJSONResults,
		UI:            l,
		JSON:          s.JSON,
		Prefix:        s.Pattern,
	})
	if err != nil {
		return errors.Wrapf(err, "error listing packages")
	}

	return nil
}
