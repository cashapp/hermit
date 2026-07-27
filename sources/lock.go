package sources

import (
	"context"
	"sync"
	"time"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util/flock"
)

// DefaultLockTimeout is how long Hermit will wait for another process to
// finish synchronising a source before giving up.
//
// This is deliberately much longer than the global state lock timeout
// (--lock-timeout, default 30s): the process holding this lock may be
// performing a full "git clone" of a manifest repository over a slow network.
const DefaultLockTimeout = 10 * time.Minute

// lockSuffix is appended to a source directory to derive its lock file path.
//
// Source directory names are bare hex SHA256 hashes (see util.Hash), so any
// entry containing a "." is unambiguously Hermit scratch state rather than a
// real source directory.
const lockSuffix = ".lock"

// util/flock is deliberately re-entrant per-process: if the lock file already
// records our own PID, Acquire returns a no-op release and the caller
// proceeds without actually holding the lock (see util/flock.Acquire). That
// means it will not serialise two goroutines within the same process.
//
// We serialise those here, with a plain (non-re-entrant) mutex keyed by lock
// path, before ever touching flock. This must NOT live in util/flock itself:
// state.CleanPackages deliberately re-acquires its own lock recursively
// (via removeRecursive), and a non-re-entrant mutex there would deadlock.
var (
	localLocksMu sync.Mutex
	localLocks   = map[string]*sync.Mutex{}
)

func localLock(path string) *sync.Mutex {
	localLocksMu.Lock()
	defer localLocksMu.Unlock()
	l, ok := localLocks[path]
	if !ok {
		l = &sync.Mutex{}
		localLocks[path] = l
	}
	return l
}

// acquireSyncLock takes an exclusive lock, across both processes and
// goroutines, on the source directory "dir".
//
// A timeout <= 0 is treated as DefaultLockTimeout.
//
// acquireSyncLock must never be called recursively (directly or indirectly)
// for the same "dir" from within the same process, as the process-local
// mutex it uses is not re-entrant.
func acquireSyncLock(log ui.Logger, dir string, timeout time.Duration, message string) (release func() error, err error) {
	if timeout <= 0 {
		timeout = DefaultLockTimeout
	}
	path := dir + lockSuffix

	local := localLock(path)
	local.Lock()

	log.Tracef("acquiring source lock %s (timeout %s)", path, timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	releaseFlock, err := flock.Acquire(ctx, path, message)
	if err != nil {
		local.Unlock()
		return nil, errors.Wrapf(err, "failed to acquire source lock %s", path)
	}
	return func() error {
		defer local.Unlock()
		return releaseFlock()
	}, nil
}
