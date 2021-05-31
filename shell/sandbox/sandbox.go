// Package sandbox provides a loosely sandboxed shell interpreter for running
// Hermit hooks in a slightly safer manner.
package sandbox

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// An Option for altering how the sandbox is run.
type Option func(*Sandbox)

// ErrSandboxViolation is returned when a script violates sandbox constraints.
var ErrSandboxViolation = errors.New("sandbox violation")

// Path adds to the sandboxes $PATH.
func Path(path ...string) Option {
	return func(c *Sandbox) {
		c.path = append(c.path, path...)
	}
}

// Allow access to these additional directories.
func Allow(dir ...string) Option {
	return func(s *Sandbox) {
		s.allow = append(s.allow, dir...)
	}
}

// Sandbox shell script evaluation under a root.
//
// The sandbox prevents writes outside the root directory, and restricts what
// can be executed to:
//
// - A minimal set of builtin emulated POSIX utilities (ls, rm, ln, etc.)
// - Binaries under "root".
// - Binaries in the $PATH as provided vi Path().
type Sandbox struct {
	allow  []string
	path   []string
	runner *interp.Runner
}

// New creates a Sandbox at root, which will also be the initial working directory.
func New(root string, options ...Option) (*Sandbox, error) {
	sandbox := &Sandbox{
		allow: []string{root},
	}
	for _, option := range options {
		option(sandbox)
	}
	runner, err := interp.New(
		interp.Dir(root),
		interp.StdIO(os.Stdin, os.Stdout, os.Stderr),
		interp.OpenHandler(sandbox.openHandler),
		interp.ExecHandler(sandbox.execHandler),
		interp.Params("-e"),
		interp.Env(expand.FuncEnviron(func(s string) string {
			return ""
		})),
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	sandbox.runner = runner
	return sandbox, nil
}

// Eval a shell script within the Sandbox.
func (s *Sandbox) Eval(script string) error {
	node, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	if err != nil {
		return errors.WithStack(err)
	}
	s.runner.Reset()
	err = s.runner.Run(context.Background(), node)
	return errors.WithStack(err)
}

func (s *Sandbox) execHandler(ctx context.Context, args []string) error {
	cctx := cmdCtx{s, interp.HandlerCtx(ctx), s.runner}
	ok, err := runBuiltinCmd(cctx, args)
	if !ok {
		return errors.Errorf("unsupported command %q", args[0])
	}
	return errors.WithStack(err)
}

func (s *Sandbox) openHandler(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
	cctx := cmdCtx{s, interp.HandlerCtx(ctx), s.runner}
	path, err := cctx.Sanitise(path)
	if err != nil {
		return nil, err
	}
	return os.OpenFile(path, flag, perm)
}
