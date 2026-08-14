package sources

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

// CommandRunner abstracts the git operations used to synchronise a source.
type CommandRunner interface {
	RunInDir(log *ui.Task, dir string, args ...string) error
	CloneInDir(log *ui.Task, dir string, source SourceURI, dest string) error
}

// RealCommandRunner runs git operations through Hermit's system command path.
type RealCommandRunner struct{}

func (r *RealCommandRunner) RunInDir(log *ui.Task, dir string, args ...string) error {
	return errors.WithStack(util.RunSystemInDir(log, dir, args...))
}

func (r *RealCommandRunner) CloneInDir(log *ui.Task, dir string, source SourceURI, dest string) error {
	args := util.GitArgs("clone", "--depth=1", "--")
	return errors.WithStack(util.RunSystemInDirWithSource(log, dir, args, source, dest))
}

// GitSource is a manifest source backed by a git repository.
type GitSource struct {
	fs        *uriFS
	sourceDir string
	path      string
	runner    CommandRunner
}

// NewGitSource returns a new GitSource
func NewGitSource(uri SourceURI, sourceDir string, runner CommandRunner) *GitSource {
	key := util.Hash(uri.Get())
	path := filepath.Join(sourceDir, key)
	return &GitSource{&uriFS{
		uri: uri,
		FS:  os.DirFS(path),
	}, sourceDir, path, runner}
}

func (s *GitSource) Sync(p *ui.UI, force bool) (bool, error) {
	info, _ := os.Stat(s.path)
	task := p.Task(s.fs.uri.String())
	if info == nil || force || time.Since(info.ModTime()) >= SyncFrequency {
		err := s.ensureSourcesDirExists()
		if err != nil {
			return false, errors.WithStack(err)
		}

		err = syncGit(task, s.sourceDir, s.fs.uri, s.path, s.runner)
		// If the sync failed while the repo had already been cloned, log a warning
		// If the repo has not yet been cloned, fail.
		if err != nil {
			if info != nil {
				task.Warnf("git sync failed: %s", err)
				return false, nil
			}
			return false, errors.Wrap(err, "git sync failed")
		}
		return true, nil
	}
	task.Debugf("Update skipped, updated within the last %s", SyncFrequency)
	return false, nil
}

func (s *GitSource) URI() SourceURI {
	return s.fs.uri
}

func (s *GitSource) Bundle() fs.FS {
	return s.fs
}

func (s *GitSource) ensureSourcesDirExists() error {
	if err := os.MkdirAll(s.sourceDir, 0700); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Atomically clone git repo.
func syncGit(b *ui.Task, dir string, source SourceURI, finalDest string, runner CommandRunner) (err error) {
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
		err = runner.RunInDir(b, finalDest, util.GitArgs("pull")...)
		if err == nil {
			return nil
		}
		// If pull fails, assume the repo is corrupted and just try and re-clone it.
	}
	// No git repo, clone down to temporary directory.
	dest, err := os.MkdirTemp(dir, filepath.Base(finalDest)+"-*")
	if err != nil {
		return errors.WithStack(err)
	}
	defer os.RemoveAll(dest)
	if err = runner.CloneInDir(b, dest, source, dest); err != nil {
		return errors.WithStack(err)
	}
	_ = os.RemoveAll(finalDest)
	// And finally, rename it into place.
	if err = os.Rename(dest, finalDest); err != nil && !os.IsExist(err) { // Prevent races.
		return errors.WithStack(err)
	}

	return nil
}
