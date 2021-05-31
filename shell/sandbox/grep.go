package sandbox

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/pkg/errors"
)

type grepCmd struct {
	Invert  bool     `short:"v" help:"Invert match."`
	List    bool     `short:"l" help:"List matching filenames."`
	Pattern string   `arg:"" help:"Pattern to match."`
	Files   []string `arg:"" optional:"" help:"Files to search."`
}

func (g *grepCmd) Run(ctx cmdCtx) error {
	re, err := regexp.CompilePOSIX(g.Pattern)
	if err != nil {
		return errors.WithStack(err)
	}
	if len(g.Files) == 0 {
		return errors.WithStack(g.grep(ctx, re, "-", ctx.Stdin))
	}
	for _, file := range g.Files {
		file, err = ctx.Sanitise(file)
		if err != nil {
			return errors.Wrap(err, "grep")
		}
		r, err := os.Open(file)
		if err != nil {
			return errors.WithStack(err)
		}
		err = g.grep(ctx, re, file, r)
		_ = r.Close()
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (g *grepCmd) grep(ctx cmdCtx, re *regexp.Regexp, filename string, r io.Reader) error {
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := s.Bytes()
		if re.Find(line) == nil {
			continue
		}
		if g.List {
			fmt.Fprintln(ctx.Stdout, filename)
			return nil
		}
		fmt.Fprintln(ctx.Stdout, string(line))
	}
	fmt.Fprint(ctx.Stdout)
	return errors.WithStack(s.Err())
}
