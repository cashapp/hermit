package sandbox

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/pkg/errors"
	"mvdan.cc/sh/v3/interp"
)

var builtins = map[string]builtinCmd{
	"ls":    &lsCmd{},
	"mkdir": &mkdirCmd{},
	"rm":    &rmCmd{},
	"cat":   &catCmd{},
	"grep":  &grepCmd{},
}

type cmdCtx struct {
	*Sandbox
	interp.HandlerContext
	runner *interp.Runner
}

// Sanitise a path within the sandbox.
func (c *cmdCtx) Sanitise(path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(c.Dir, path)
	}
	for _, dir := range c.allow {
		if strings.HasPrefix(path, dir) {
			return path, nil
		}
	}
	return "", errors.Wrap(ErrSandboxViolation, path)
}

type builtinCmd interface {
	Run(bctx cmdCtx) error
}

func runBuiltinCmd(bctx cmdCtx, args []string) (present bool, err error) {
	// We make Kong report exit() via a panic so it doesn't continue.
	defer func() {
		value := recover()
		if value != nil {
			present = true
			switch value := value.(type) {
			case int:
				if value != 0 {
					err = interp.NewExitStatus(uint8(value))
				}
			case error:
				err = value
			default:
				err = errors.Errorf("%s", value)
			}
		}
	}()
	// fmt.Fprintf(bctx.Stderr, "+ %s\n", shellquote.Join(args...))
	factory, ok := builtins[args[0]]
	if !ok {
		return false, nil
	}
	cmd := reflect.New(reflect.TypeOf(factory).Elem()).Interface().(builtinCmd)
	exitStatus := 0
	_, err = kong.Must(cmd,
		kong.Exit(func(i int) { panic(i) }),
		kong.ShortUsageOnError(),
		kong.Name(args[0]),
	).Parse(args[1:])
	if err != nil {
		fmt.Fprintf(bctx.Stderr, "%s: %s\n", args[0], err)
		return true, interp.NewExitStatus(1)
	}
	if exitStatus != 0 {
		return true, nil
	}
	err = cmd.Run(bctx)
	if err != nil {
		fmt.Fprintf(bctx.Stderr, "%s: %s\n", args[0], err)
		return true, interp.NewExitStatus(1)
	}
	return true, nil
}
