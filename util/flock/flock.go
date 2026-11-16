package flock

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
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

// heldLock is a file lock held by this process, shared by every in-process holder.
type heldLock struct {
	holders int
	unlock  func() error
}

// Locks held by this process, keyed by absolute path. Not inferred from the lock file's PID,
// because processes in different PID namespaces can share a PID.
var (
	heldMu sync.Mutex
	held   = map[string]*heldLock{}
)

// Acquire a lock on the given path, storing the current PID and a message in the lock file.
//
// The lock is released when the returned function is called.
//
// If this process already holds the lock, Acquire shares it process-wide (not per goroutine) without updating the
// message, and the file lock is released when the last holder releases.
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
		release, err := acquireShared(absPath, message) //nolint
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, ErrLocked) {
			return nil, errors.Wrapf(err, "failed to acquire lock %s", absPath)
		}

		// Only used to report the holder; errors leave it empty.
		pidBytes, _ := os.ReadFile(absPath)
		pid := pidFile{}
		_ = json.Unmarshal(pidBytes, &pid)

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

// acquireShared joins this process's hold on path, or takes the file lock if this process does not hold it.
func acquireShared(absPath, message string) (release func() error, err error) {
	heldMu.Lock()
	defer heldMu.Unlock()
	lock, ok := held[absPath]
	if !ok {
		// Taken under heldMu so goroutines in this process never race each other for the file lock.
		unlock, err := acquire(absPath, message)
		if err != nil {
			return nil, err
		}
		lock = &heldLock{unlock: unlock}
		held[absPath] = lock
	}
	lock.holders++
	// Once, so a repeated call cannot release a hold that belongs to another caller.
	return sync.OnceValue(func() error {
		heldMu.Lock()
		defer heldMu.Unlock()
		lock.holders--
		if lock.holders > 0 {
			return nil
		}
		delete(held, absPath)
		return lock.unlock()
	}), nil
}

func acquire(path, message string) (release func() error, err error) {
	pid := os.Getpid()
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
