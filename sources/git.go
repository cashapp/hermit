package sources

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/redact"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
	"github.com/cashapp/hermit/util/flock"
)

// syncLockTimeout is how long a sync waits for another process to finish
// cloning or pulling the same source. It is longer than the state lock timeout
// because a first clone of a large manifest repository can be slow.
const syncLockTimeout = 5 * time.Minute

// GitSource is a new Source based on a git repo
type GitSource struct {
	fs        *uriFS
	remote    redact.URL
	sourceDir string
	path      string
	runner    util.CommandRunner
}

// NewGitSource returns a new GitSource
func NewGitSource(uri redact.URL, sourceDir string, runner util.CommandRunner) *GitSource {
	key := util.Hash(uri.Reveal())
	path := filepath.Join(sourceDir, key)
	return &GitSource{
		fs: &uriFS{
			uri: uri.String(),
			FS:  os.DirFS(path),
		},
		remote:    uri,
		sourceDir: sourceDir,
		path:      path,
		runner:    runner,
	}
}

func (s *GitSource) Sync(p *ui.UI, force bool) (bool, error) {
	info, _ := os.Stat(s.path)
	task := p.Task(s.fs.uri)
	if info != nil && !force && time.Since(info.ModTime()) < SyncFrequency {
		task.Debugf("Update skipped, updated within the last %s", SyncFrequency)
		return false, nil
	}
	if err := s.ensureSourcesDirExists(); err != nil {
		return false, errors.WithStack(err)
	}

	// Hermit processes share the checkout. Unserialised syncs clone or pull it
	// concurrently, and a failed pull or a clone replaces the checkout that other
	// processes are reading manifests from.
	waitStart := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), syncLockTimeout)
	release, err := flock.Acquire(ctx, s.path+".lock", "syncing "+s.fs.uri)
	cancel()
	if err != nil {
		return false, errors.Wrap(err, "failed to acquire the source sync lock")
	}
	defer release() //nolint:errcheck

	// A sync that finished while we waited is as fresh as one we would run now.
	info, _ = os.Stat(s.path)
	if info != nil && info.ModTime().After(waitStart) {
		task.Debugf("Update skipped, synced by another process")
		return true, nil
	}

	err = syncGit(task, s.sourceDir, s.remote, s.path, s.runner)
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

func (s *GitSource) URI() string {
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
func syncGit(b *ui.Task, dir string, source redact.URL, finalDest string, runner util.CommandRunner) (err error) {
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
		err = runner.RunInDir(b, finalDest, redact.Args(util.GitArgs("pull")...)...)
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
	cloneArgs := append(redact.Args(util.GitArgs("clone", "--depth=1", "--")...), source, redact.Plain(dest))
	if err = runner.RunInDir(b, dest, cloneArgs...); err != nil {
		return errors.WithStack(err)
	}
	// Other processes read manifests without the sync lock, so move the old
	// checkout aside rather than deleting it in place. This shrinks the window in
	// which no checkout exists from a recursive delete to a single rename.
	old := dest + ".old"
	if err = os.Rename(finalDest, old); err != nil && !os.IsNotExist(err) {
		return errors.WithStack(err)
	}
	defer os.RemoveAll(old)
	if err = os.Rename(dest, finalDest); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
