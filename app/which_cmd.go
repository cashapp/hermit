package app

import (
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type whichCmd struct {
	Binary string `arg:"" help:"Binary name or path to a Hermit binary symlink."`
}

func (w *whichCmd) Run(ctx *kong.Context, l *ui.UI, env *hermit.Env, sta *state.State) error {
	binary := w.Binary
	if filepath.Base(binary) == binary {
		binary = filepath.Join(env.BinDir(), binary)
	}
	if _, err := os.Lstat(binary); errors.Is(err, os.ErrNotExist) {
		ctx.Exit(1)
		return nil
	} else if err != nil {
		return errors.WithStack(err)
	}
	pkg, binary, err := env.ResolveLink(l, binary)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := pkg.EnsureSupported(); err != nil {
		return errors.WithStack(err)
	}
	if err := sta.CacheAndUnpack(l.Task(pkg.Reference.String()), pkg); err != nil {
		return errors.WithStack(err)
	}
	resolved, err := pkg.ResolveBinary(binary)
	if err != nil {
		return errors.WithStack(err)
	}
	l.Printf("%s\n", resolved)
	return nil
}
