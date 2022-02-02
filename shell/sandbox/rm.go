package sandbox

import (
	"os"

	"github.com/pkg/errors"
)

type rmCmd struct {
	Recursive bool `short:"r" help:"Recursively delete."`
	Force     bool `short:"f" help:"Force deletion of read-only files."`

	Paths []string `arg:"" help:"Paths to delete."`
}

func (r *rmCmd) Run(bctx cmdCtx) error {
	for _, path := range r.Paths {
		var err error
		path, err = bctx.Sanitise(path)
		if err != nil {
			return err
		}
		if r.Recursive {
			if err = os.RemoveAll(path); err != nil {
				return errors.WithStack(err)
			}
		} else {
			if err = os.Remove(path); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	return nil
}
