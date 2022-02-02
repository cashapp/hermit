package sandbox

import (
	"io"
	"os"

	"github.com/pkg/errors"
)

type catCmd struct {
	Paths []string `arg:"" optional:"" help:"Files to cat, if any."`
}

func (c *catCmd) Run(ctx cmdCtx) error {
	if len(c.Paths) == 0 {
		_, err := io.Copy(ctx.Stdout, ctx.Stdin)
		return errors.WithStack(err)
	}
	for _, path := range c.Paths {
		var err error
		path, err = ctx.Sanitise(path)
		if err != nil {
			return errors.WithStack(err)
		}
		r, err := os.Open(path)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = io.Copy(ctx.Stdout, r)
		_ = r.Close()
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
