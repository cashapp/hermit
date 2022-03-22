package sources

import (
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

// GitSource is a new Source based on a git repo
type GitSource struct {
	fs        *uriFS
	sourceDir string
	path      string
}

// NewGitSource returns a new GitSource
func NewGitSource(uri string, sourceDir string) *GitSource {
	key := util.Hash(uri)
	path := filepath.Join(sourceDir, key)
	return &GitSource{&uriFS{
		uri: uri,
		FS:  os.DirFS(path),
	}, sourceDir, path}
}

func (s *GitSource) Sync(p *ui.UI, force bool) error { // nolint: golint
	info, _ := os.Stat(s.path)
	task := p.Task(s.fs.uri)
	if info == nil || force || time.Since(info.ModTime()) >= SyncFrequency {
		err := s.ensureSourcesDirExists()
		if err != nil {
			return errors.WithStack(err)
		}

		err = syncGit(task, s.sourceDir, s.fs.uri, s.path)
		// If the sync failed while the repo had already been cloned, log a warning
		// If the repo has not yet been cloned, fail.
		if err != nil {
			if info != nil {
				task.Warnf("git sync failed: %s", err)
			} else {
				return errors.Wrap(err, "git sync failed")
			}
		}
	} else {
		task.Debugf("Update skipped, updated within the last %s", SyncFrequency)
	}
	return nil
}

func (s *GitSource) URI() string { // nolint: golint
	return s.fs.uri
}

func (s *GitSource) Bundle() fs.FS { // nolint: golint
	return s.fs
}

func (s *GitSource) ensureSourcesDirExists() error {
	if err := os.MkdirAll(s.sourceDir, 0700); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Atomically clone git repo.
func syncGit(b *ui.Task, dir, source, finalDest string) (err error) {
	task := b.SubProgress("sync", 1)
	defer func() {
		task.Done()
		now := time.Now()
		if err == nil {
			err = errors.WithStack(os.Chtimes(finalDest, now, now))
		}
	}()
	// First, if a git repo exists, just pull.
	info, _ := os.Stat(filepath.Join(finalDest, ".git"))
	if info != nil {
		err = util.RunInDir(b, finalDest, "git", "pull")
		if err == nil {
			return nil
		}
		// If pull fails, assume the repo is corrupted and just try and re-clone it.
	}
	// No git repo, clone down to temporary directory.
	_ = os.RemoveAll(finalDest)
	dest, err := ioutil.TempDir(dir, filepath.Base(finalDest)+"-*")
	if err != nil {
		return errors.WithStack(err)
	}
	defer os.RemoveAll(dest)
	if err = util.RunInDir(b, dest, "git", "clone", "--depth=1", source, dest); err != nil {
		return errors.WithStack(err)
	}
	// And finally, rename it into place.
	if err = os.Rename(dest, finalDest); err != nil && !os.IsExist(err) { // Prevent races.
		return errors.WithStack(err)
	}

	return nil
}
