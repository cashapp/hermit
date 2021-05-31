package sandbox

import (
	"os"

	"github.com/pkg/errors"
)

type mkdirCmd struct {
	Mode         os.FileMode `short:"m" help:"Set the file permission bits of the final created directory to the specified mode."`
	Intermediate bool        `short:"p" help:"Create intermediate directories as needed."`
	Dirs         []string    `arg:"" help:"Directories to create"`
}

func (m *mkdirCmd) Run(ctx cmdCtx) error {
	mode := m.Mode
	if mode == 0 {
		mode = 0777
	}
	for _, dir := range m.Dirs {
		var err error
		dir, err = ctx.Sanitise(dir)
		if err != nil {
			return err
		}
		if m.Intermediate {
			if err := os.MkdirAll(dir, mode); err != nil {
				return errors.Wrap(err, dir)
			}
		} else {
			if err := os.Mkdir(dir, mode); err != nil {
				return errors.Wrap(err, dir)
			}
		}
	}
	return nil
}
