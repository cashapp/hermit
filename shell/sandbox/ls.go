package sandbox

import (
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/pkg/errors"
)

type lsCmd struct {
	All  bool `short:"a" help:"List all files."`
	Long bool `short:"l" help:"Long listing."`

	Paths []string `arg:"" optional:"" help:"Paths to list."`
}

func (l *lsCmd) Run(ctx cmdCtx) error {
	paths := l.Paths
	if len(paths) == 0 {
		paths = append(paths, ctx.Dir)
	}
	w := tabwriter.NewWriter(ctx.Stdout, 4, 4, 1, ' ', 0)
	for _, path := range paths {
		var err error
		path, err = ctx.Sanitise(path)
		if err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err != nil {
			return errors.WithStack(err)
		}
		var entries []fs.FileInfo
		if info.IsDir() {
			entries, err = ioutil.ReadDir(path)
			if err != nil {
				return errors.WithStack(err)
			}
			// Synthesise . and ..
			if l.All {
				dotdot, err := os.Stat(filepath.Join(path, ".."))
				if err != nil {
					return errors.WithStack(err)
				}
				entries = append([]fs.FileInfo{
					&renamedFileInfo{info, "."},
					&renamedFileInfo{dotdot, ".."},
				}, entries...)
			}
		} else {
			entries = append(entries, info)
		}
		for _, entry := range entries {
			if !l.All && strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if !l.Long {
				fmt.Fprintln(w, entry.Name())
				continue
			}
			var (
				size      uint64
				username  = "user"
				groupname = "group"
			)
			if stat, ok := entry.Sys().(*syscall.Stat_t); ok {
				usr, err := user.LookupId(fmt.Sprintf("%d", stat.Uid))
				if err != nil {
					return errors.WithStack(err)
				}
				group, err := user.LookupGroupId(fmt.Sprintf("%d", stat.Gid))
				if err != nil {
					return errors.WithStack(err)
				}
				username = usr.Username
				groupname = group.Name
				size = uint64(stat.Nlink) // nolint

			}
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%d\t%s\t%s\n",
				entry.Mode(), size, username, groupname, entry.Size(),
				entry.ModTime().Format("2 Jan 15:04"),
				entry.Name())
		}
	}
	return w.Flush()
}

type renamedFileInfo struct {
	fs.FileInfo
	name string
}

func (r *renamedFileInfo) Name() string {
	return r.name
}
