package sources

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

// fsTimeGranularity is the coarsest mtime resolution we expect from the
// filesystems Hermit runs on (eg. HFS+ stores whole seconds), used as slack
// when comparing timestamps taken before and after acquiring the sync lock.
const fsTimeGranularity = time.Second

// Suffixes/prefixes used for Hermit scratch state living alongside source
// directories. A real source directory name is a bare hex SHA256 hash (see
// util.Hash), so anything containing these is unambiguously scratch state.
const (
	tmpInfix        = ".tmp-"                 // in-progress clone: <hash>.tmp-XXXXXXXX
	asideSuffix     = util.DirSwapAsideSuffix // previous tree, mid-swap, pending deletion
	legacyTmpInfix  = "-"                     // clone temp dirs created by older Hermit versions: <hash>-XXXXXXXX
	staleScratchAge = 24 * time.Hour
)

// GitSource is a new Source based on a git repo
type GitSource struct {
	fs          *uriFS
	sourceDir   string
	path        string
	runner      util.CommandRunner
	lockTimeout time.Duration
}

// NewGitSource returns a new GitSource
func NewGitSource(uri, sourceDir string, runner util.CommandRunner) *GitSource {
	return NewGitSourceWithLockTimeout(uri, sourceDir, runner, DefaultLockTimeout)
}

// NewGitSourceWithLockTimeout returns a new GitSource with an explicit
// timeout for the lock acquired around synchronisation.
//
// A timeout <= 0 is treated as DefaultLockTimeout. This is primarily useful
// for tests.
func NewGitSourceWithLockTimeout(uri, sourceDir string, runner util.CommandRunner, lockTimeout time.Duration) *GitSource {
	key := util.Hash(uri)
	path := filepath.Join(sourceDir, key)
	return &GitSource{&uriFS{
		uri: uri,
		FS:  os.DirFS(path),
	}, sourceDir, path, runner, lockTimeout}
}

func (s *GitSource) Sync(p *ui.UI, force bool) (bool, error) {
	task := p.Task(s.fs.uri)

	info, _ := os.Stat(s.path)
	if info != nil && !force && time.Since(info.ModTime()) < SyncFrequency {
		task.Debugf("Update skipped, updated within the last %s", SyncFrequency)
		return false, nil
	}

	if err := s.ensureSourcesDirExists(); err != nil {
		return false, errors.WithStack(err)
	}

	// Note the time *before* we start waiting for the lock: if, once we hold
	// it, the directory's mtime is at or after this instant, another process
	// finished synchronising it while we were waiting, and there is nothing
	// left for us to do.
	requestedAt := time.Now()
	release, err := acquireSyncLock(task, s.path, s.lockTimeout, fmt.Sprintf("synchronising source %s", s.fs.uri))
	if err != nil {
		if info != nil {
			// We already have a (possibly stale) usable copy. Don't fail the
			// command just because we couldn't get exclusive access to
			// refresh it.
			task.Warnf("could not lock source for syncing, using existing copy: %s", err)
			return false, nil
		}
		return false, errors.Wrap(err, "failed to sync sources")
	}
	defer release() //nolint:errcheck

	// Double-checked locking: re-stat now that we hold the lock.
	postLockInfo, _ := os.Stat(s.path)
	if syncedSince(postLockInfo, requestedAt) {
		task.Debugf("Update skipped, synchronised by another process")
		return true, nil
	}

	err = syncGit(task, s.sourceDir, s.fs.uri, s.path, s.runner)
	if err != nil {
		// If the sync failed while the repo had already been cloned (using
		// the up to date, post-lock information), log a warning. If the repo
		// has not yet been cloned, fail.
		if postLockInfo != nil {
			task.Warnf("git sync failed: %s", err)
			return false, nil
		}
		return false, errors.Wrap(err, "git sync failed")
	}
	return true, nil
}

// syncedSince reports whether "info" (the result of stat-ing a source
// directory) shows it was successfully synced at or after "since", allowing
// fsTimeGranularity of slack for filesystems that only store whole-second
// mtimes (eg. HFS+).
func syncedSince(info os.FileInfo, since time.Time) bool {
	return info != nil && !info.ModTime().Add(fsTimeGranularity).Before(since)
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
//
// The caller MUST hold the sync lock for finalDest.
func syncGit(b *ui.Task, dir, source, finalDest string, runner util.CommandRunner) (err error) {
	task := b.SubProgress("sync", 1)
	defer func() {
		task.Done()
		now := time.Now()
		if err == nil {
			err = errors.WithStack(os.Chtimes(finalDest, now, now))
		}
	}()

	removeStaleScratchDirs(b, dir, finalDest)

	// First, if a git repo exists, just pull.
	info, _ := os.Stat(filepath.Join(finalDest, ".git"))
	if info != nil {
		err = runner.RunInDir(b, finalDest, "git", "pull")
		if err == nil {
			return nil
		}
		// If pull fails, assume the repo is corrupted and just try and re-clone it.
	}
	// No git repo, clone down to temporary directory.
	dest, err := os.MkdirTemp(dir, filepath.Base(finalDest)+tmpInfix+"*")
	if err != nil {
		return errors.WithStack(err)
	}
	defer os.RemoveAll(dest)
	if err = runner.RunInDir(b, dest, "git", "clone", "--depth=1", source, dest); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(swapDir(dest, finalDest))
}

// swapDir atomically (from the point of view of an unlocked reader) replaces
// finalDest with src. See util.SwapDir for how.
//
// Readers in other Hermit processes do not take the sync lock, so this
// matters here specifically to avoid the ENOENT-during-clone window that
// makes every package look unknown while a large source tree is being
// replaced.
//
// The caller MUST hold the sync lock for finalDest.
func swapDir(src, finalDest string) error {
	return util.SwapDir(src, finalDest)
}

// removeStaleScratchDirs removes leftover clone/swap scratch directories from
// Hermit processes that were killed mid-sync (eg. SIGKILL, which the
// "defer os.RemoveAll" in syncGit cannot run for), including the "<hash>-XXXX"
// form used by Hermit versions prior to the introduction of source locking.
//
// The caller MUST hold the sync lock for finalDest. This is best-effort;
// errors are ignored.
func removeStaleScratchDirs(log ui.Logger, dir, finalDest string) {
	base := filepath.Base(finalDest)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == base || strings.HasSuffix(name, lockSuffix) {
			continue
		}
		if !strings.HasPrefix(name, base+tmpInfix) &&
			name != base+asideSuffix &&
			!strings.HasPrefix(name, base+legacyTmpInfix) {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < staleScratchAge {
			// Generous age threshold: an older, unlocked Hermit binary may
			// still be actively cloning into one of these.
			continue
		}
		log.Debugf("removing stale source scratch directory %s", name)
		_ = os.RemoveAll(filepath.Join(dir, name))
	}
}
