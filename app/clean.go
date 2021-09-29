package app

import (
	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/ui"
	"github.com/pkg/errors"
)

type cleanCmd struct {
	Bin       bool `short:"b" help:"Clean links out of the local bin directory."`
	Packages  bool `short:"p" help:"Clean all extracted packages."`
	Cache     bool `short:"c" help:"Clean download cache."`
	Transient bool `short:"a" help:"Clean everything transient (packages, cache)."`
}

func (c *cleanCmd) Run(l *ui.UI, env *hermit.Env) error {
	var mask hermit.CleanMask
	if c.Bin {
		mask |= hermit.CleanBin
	}
	if c.Packages {
		mask |= hermit.CleanPackages
	}
	if c.Cache {
		mask |= hermit.CleanCache
	}
	if c.Transient {
		mask = hermit.CleanTransient
	}
	if mask == 0 {
		return errors.New("no targets to clean, try --help")
	}
	return env.Clean(l, mask)
}
