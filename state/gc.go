package state

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/ui"
)

// Usage is a directory's mtime, the only record cheap enough to update on every exec.
// Throttling keeps frequent execs to a stat rather than a metadata write.
const usageGranularity = time.Hour

// RecordUsage marks an unpacked package as used, protecting it from GC.
func (s *State) RecordUsage(pkg *manifest.Package) {
	// Packages unpacked outside the state directory are never collected, so leave them alone.
	if filepath.Dir(pkg.Dest) != s.pkgDir {
		return
	}
	Touch(pkg.Dest)
}

// Touch marks a directory in the state directory as used, protecting it from GC.
// Errors only make the directory eligible for GC sooner, so they are ignored.
func Touch(path string) {
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) < usageGranularity {
		return
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}

// GC removes everything in the state directory not used since maxAge ago.
// Usage is only recorded when Hermit runs, so long-idle activated shells may lose linked binaries.
func (s *State) GC(l *ui.UI, maxAge time.Duration) error {
	release, err := s.acquireLock(l, "garbage collecting")
	if err != nil {
		return errors.WithStack(err)
	}
	defer release() //nolint:errcheck

	cutoff := time.Now().Add(-maxAge)
	// Phases run in order because binary links and metadata are collected once their package is gone.
	phases := []func(cutoff time.Time) ([]string, error){
		s.unusedPackages,
		s.danglingBinaryLinks,
		s.orphanedMetadata,
		s.unusedLinkFarms,
		s.unusedDownloads,
		// Sources are synced, and so modified, at least daily while in use.
		func(cutoff time.Time) ([]string, error) { return entriesOlderThan(s.sourcesDir, cutoff) },
	}
	// The gc task holds the units not yet removed and hands one to each removal's task, so the
	// total only grows when a phase starts while the bar names the directory being removed.
	var task *ui.Task
	remaining := 0
	for _, find := range phases {
		paths, err := find(cutoff)
		if err != nil {
			return errors.WithStack(err)
		}
		if len(paths) == 0 {
			continue
		}
		remaining += len(paths)
		if task == nil {
			task = l.Progress("gc", remaining)
		} else {
			task.Size(remaining)
		}
		for _, path := range paths {
			rel, err := filepath.Rel(s.root, path)
			if err != nil {
				return errors.WithStack(err)
			}
			remaining--
			task.Size(remaining)
			// Displayed after the gc task's own label.
			removal := l.Progress(rel, 1)
			if err := s.removeRecursive(removal, path); err != nil {
				return errors.WithStack(err)
			}
			removal.Done()
		}
	}
	return nil
}

// unusedPackages includes staging directories left by interrupted unpacks.
func (s *State) unusedPackages(cutoff time.Time) ([]string, error) {
	entries, err := readDir(s.pkgDir)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var paths []string
	for _, entry := range entries {
		// The running Hermit binary lives here.
		if !strings.HasPrefix(entry.Name(), "hermit@") && isOlder(entry, cutoff) {
			paths = append(paths, filepath.Join(s.pkgDir, entry.Name()))
		}
	}
	return paths, nil
}

// danglingBinaryLinks finds per-package link directories containing a link to a missing binary.
// CacheAndUnpack recreates them when the package is next used.
func (s *State) danglingBinaryLinks(time.Time) ([]string, error) {
	entries, err := readDir(s.binaryDir)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var paths []string
	for _, entry := range entries {
		dir := filepath.Join(s.binaryDir, entry.Name())
		links, err := os.ReadDir(dir)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		dangling := len(links) == 0
		for _, link := range links {
			if _, err := os.Stat(filepath.Join(dir, link.Name())); err != nil {
				dangling = true
				break
			}
		}
		if dangling {
			paths = append(paths, dir)
		}
	}
	return paths, nil
}

// orphanedMetadata finds update-check records of packages that are no longer present.
// Binary links reveal packages unpacked outside the state directory.
func (s *State) orphanedMetadata(cutoff time.Time) ([]string, error) {
	dir := s.dao.MetadataDir()
	entries, err := readDir(dir)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var paths []string
	for _, entry := range entries {
		ref, ok := strings.CutSuffix(entry.Name(), ".etag")
		if !ok || strings.HasPrefix(ref, "hermit@") || !isOlder(entry, cutoff) {
			continue
		}
		if !exists(filepath.Join(s.pkgDir, ref)) && !exists(filepath.Join(s.binaryDir, ref)) {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	return paths, nil
}

// unusedLinkFarms finds link farms not activated since the cutoff, returning an environment's
// whole directory when none of its farms are in use.
func (s *State) unusedLinkFarms(cutoff time.Time) ([]string, error) {
	entries, err := readDir(s.envsDir)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var paths []string
	for _, entry := range entries {
		dir := filepath.Join(s.envsDir, entry.Name())
		farms, err := readDir(dir)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		unused, err := entriesOlderThan(dir, cutoff)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if len(unused) == len(farms) {
			paths = append(paths, dir)
		} else {
			paths = append(paths, unused...)
		}
	}
	return paths, nil
}

// unusedDownloads finds downloads not written since the cutoff. Downloads are only read to
// unpack a package, which then no longer needs them.
func (s *State) unusedDownloads(cutoff time.Time) ([]string, error) {
	shards, err := readDir(s.cacheDir)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var paths []string
	for _, shard := range shards {
		// Shards are left in place because downloads create files in them without the lock.
		unused, err := entriesOlderThan(filepath.Join(s.cacheDir, shard.Name()), cutoff)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		paths = append(paths, unused...)
	}
	return paths, nil
}

// entriesOlderThan returns the entries of dir last modified before cutoff.
func entriesOlderThan(dir string, cutoff time.Time) ([]string, error) {
	entries, err := readDir(dir)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var paths []string
	for _, entry := range entries {
		if isOlder(entry, cutoff) {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	return paths, nil
}

func readDir(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return entries, errors.WithStack(err)
}

// isOlder reports whether entry was last modified before cutoff. An entry that vanished is
// reported as recent so that it is left alone.
func isOlder(entry os.DirEntry, cutoff time.Time) bool {
	info, err := entry.Info()
	if err != nil {
		return false
	}
	return info.ModTime().Before(cutoff)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
