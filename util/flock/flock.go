package flock

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/cashapp/hermit/errors"
	"golang.org/x/sys/unix"
)

var (
	ErrLocked  = errors.New("locked")
	ErrTimeout = errors.New("lock timed out")
)

type pidFile struct {
	PID     int    `json:"pid"`
	Message string `json:"message"`
}

// Used for testing to allow mocking of os.Getpid.
var getPID = os.Getpid

// Acquire a lock on the given path, storing the current PID and a message in the lock file.
//
// The lock is released when the returned function is called.
//
// If the lock is held by the current process, Acquire will return a no-op release function and the message WILL NOT be
// updated.
//
// If the lock is held by another process, Acquire will block until the lock is released or the context is cancelled.
//
// The file is NOT deleted on release; doing so creates a race condition that allows multiple processes to acquire
// the same lock.
func Acquire(ctx context.Context, path, message string) (release func() error, err error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	for {
		release, err := acquire(absPath, message) //nolint
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, ErrLocked) {
			return nil, errors.Wrapf(err, "failed to acquire lock %s", absPath)
		}

		// If our own PID is holding the lock, we can return a no-op release function.
		//
		// We can safely ignore errors here because the comparison will fail anway if the file doesn't contain our PID.
		pidBytes, _ := os.ReadFile(absPath)
		pid := pidFile{}
		_ = json.Unmarshal(pidBytes, &pid)
		if pid.PID == getPID() {
			return func() error { return nil }, nil
		}

		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, errors.Wrapf(ErrTimeout, "timed out acquiring lock %s after %s, locked by pid %v: %s", absPath, time.Since(start), pid, pid.Message)
			}
			return nil, errors.Wrapf(ctx.Err(), "context cancelled while acquiring lock %s after %s, locked by pid %v: %s", absPath, time.Since(start), pid, pid.Message)

		case <-time.After(time.Millisecond * 100):
		}
	}
}

// acquire takes the flock itself, then records our PID in the lock file's
// contents for the benefit of Acquire's own-PID re-entrancy check above.
//
// There is a small window between the LOCK_EX succeeding and the PID payload
// being written below: a concurrent Acquire call in another process that
// reads the file's contents during that window sees either an empty file
// (first-ever acquisition) or a stale PID from whoever held the lock
// previously -- never our own PID, so its own-PID check simply falls
// through to the normal wait/retry path rather than misbehaving. This is
// pre-existing and has always been benign, but it is now load-bearing:
// sources/lock.go's cross-process source lock depends on Acquire's
// re-entrancy check to avoid a second sync within the same process
// deadlocking against itself.
func acquire(path, message string) (release func() error, err error) {
	pid := getPID()
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_SYNC, 0600)
	if err != nil {
		return nil, errors.Wrapf(err, "open failed")
	}

	err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
	if err != nil {
		_ = unix.Close(fd)
		return nil, errors.Wrapf(ErrLocked, "%s", err)
	}

	payload, err := json.Marshal(pidFile{PID: pid, Message: message})
	if err != nil {
		return nil, errors.Wrapf(err, "marshal failed")
	}

	err = unix.Ftruncate(fd, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "truncate failed")
	}

	_, err = unix.Write(fd, payload)
	if err != nil {
		return nil, errors.Wrapf(err, "write failed")
	}
	return func() error {
		return errors.Join(unix.Flock(fd, unix.LOCK_UN), unix.Close(fd))
	}, nil
}
