package util

import (
	"os"
	"path/filepath"

	"github.com/cashapp/hermit/errors"
)

// DirSwapAsideSuffix is appended to finalDest to name the location its
// previous contents are moved aside to during SwapDir. Exported so callers
// that sweep up leftover scratch state (eg. after a crash mid-swap) can
// recognise these directories without duplicating the literal.
const DirSwapAsideSuffix = ".old"

// SwapDir atomically (from the point of view of an unlocked reader) replaces
// finalDest with src.
//
// Some callers have readers that check a directory's contents (or existence)
// without taking any lock -- eg. another Hermit process reading a manifest
// source, or CacheAndUnpack's pre-lock fast path checking a package's linked
// binaries. For a large directory, removing finalDest outright and
// recreating it can take long enough (or leave it transiently
// missing/partial for long enough) that a concurrent unlocked read observes
// ENOENT or an incomplete directory. Instead, the existing tree is renamed
// aside and the new one is renamed into its place. This shrinks the window
// in which finalDest does not exist from however long it takes to remove and
// repopulate a potentially large tree, down to the gap between two rename(2)
// calls in the same directory -- not zero, but small and constant-time
// regardless of tree size, and each rename itself is atomic so a reader never
// observes a partially-written directory.
//
// The caller must ensure no other goroutine or process can be concurrently
// mutating finalDest (eg. by holding an appropriate lock); SwapDir only
// protects readers, not other writers.
func SwapDir(src, finalDest string) error {
	aside := finalDest + DirSwapAsideSuffix

	// May exist already if a previous process died mid-swap. Safe to remove:
	// the caller is assumed to hold exclusive write access, so nothing is
	// relying on it any more.
	if err := os.RemoveAll(aside); err != nil {
		return errors.WithStack(err)
	}
	if err := os.Rename(finalDest, aside); err != nil && !os.IsNotExist(err) {
		return errors.WithStack(err)
	}
	if err := os.Rename(src, finalDest); err != nil {
		// Put the previous tree back so that we degrade to "stale" rather
		// than "missing".
		if rerr := os.Rename(aside, finalDest); rerr != nil && !os.IsNotExist(rerr) {
			return errors.Join(errors.WithStack(err), errors.WithStack(rerr))
		}
		return errors.WithStack(err)
	}
	// aside is no longer reachable via finalDest, and readers with an open
	// FS handle are unaffected by unlinking it, so it's safe to remove
	// synchronously here. We deliberately don't defer this to a goroutine:
	// Hermit's exec path ends in syscall.Exec, which would silently kill any
	// in-flight background cleanup and leak the directory forever.
	return errors.WithStack(os.RemoveAll(aside))
}

// RemoveAllAtomic removes dir in a way that's safe for an unlocked reader:
// dir is first renamed to a uniquely-named sibling, then removed. This closes
// the same reader-visible window SwapDir does for replacement -- a reader
// that stats or opens dir sees either the whole original tree or ENOENT,
// never a tree with some entries already unlinked out from under it.
//
// The caller must ensure no other goroutine or process can be concurrently
// mutating dir.
//
// Unlike os.RemoveAll, RemoveAllAtomic is not nil-safe for a wholly-missing
// path: it requires dir's parent directory to exist (MkdirTemp needs
// somewhere to create the sibling), and returns an error if the parent is
// itself missing. A missing dir with an existing parent is still handled --
// that case returns nil, same as os.RemoveAll.
func RemoveAllAtomic(dir string) error {
	aside, err := os.MkdirTemp(filepath.Dir(dir), filepath.Base(dir)+DirSwapAsideSuffix+"-*")
	if err != nil {
		return errors.WithStack(err)
	}
	// MkdirTemp creates aside itself; remove the placeholder so the rename
	// below can take its place.
	if err := os.Remove(aside); err != nil {
		return errors.WithStack(err)
	}
	if err := os.Rename(dir, aside); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.WithStack(err)
	}
	return errors.WithStack(os.RemoveAll(aside))
}
