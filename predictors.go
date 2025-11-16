package hermit

import (
	"github.com/posener/complete"

	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

// PackagePredictor is a shell completion predictor for all package names
type PackagePredictor struct {
	state *state.State
	env   *Env
	l     *ui.UI
}

// NewPackagePredictor returns a new PackagePredictor
func NewPackagePredictor(s *state.State, e *Env, l *ui.UI) *PackagePredictor {
	return &PackagePredictor{s, e, l}
}

func (p *PackagePredictor) Predict(args complete.Args) []string { // nolint: golint
	p.l.SetLevel(ui.LevelFatal)

	var res []string
	// if there is an error, just quietly return an empty list
	if p.env == nil {
		res, _ = p.state.SearchPrefix(p.l, args.Last)
	} else {
		res, _ = p.env.SearchPrefix(p.l, args.Last)
	}

	return res
}

// InstalledPackagePredictor is a shell completion predictor for installed package names
type InstalledPackagePredictor struct {
	env *Env
	l   *ui.UI
}

// NewInstalledPackagePredictor returns a new InstalledPackagePredictor
func NewInstalledPackagePredictor(e *Env, l *ui.UI) *InstalledPackagePredictor {
	return &InstalledPackagePredictor{e, l}
}

func (p *InstalledPackagePredictor) Predict(args complete.Args) []string { // nolint: golint
	p.l.SetLevel(ui.LevelFatal)

	// if there is an error, just quietly return an empty list
	pkgs, _ := p.env.ListInstalled(p.l)

	res := make([]string, len(pkgs))
	for i, pkg := range pkgs {
		res[i] = pkg.Reference.Name
	}
	return res
}
