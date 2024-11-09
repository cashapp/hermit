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
func Acquire(ctx context.Context, path, message string) (release func() error, err error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	for {
		release, err := acquire(absPath, message)
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, ErrLocked) {
			return nil, errors.Wrapf(err, "failed to acquire lock %s", absPath)
		}

		// If our own PID is holding the lock, we can return a no-op release function.
		//
		// We can safely ignore errors here because the comparison will fail anway if the file doesn't contain our PID.
		pidBytes, _ := os.ReadFile(absPath) //nolint:errcheck
		pid := pidFile{}
		_ = json.Unmarshal(pidBytes, &pid) //nolint:errcheck
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

	_, err = unix.Write(fd, payload)
	if err != nil {
		return nil, errors.Wrapf(err, "write failed")
	}
	return func() error {
		return errors.Join(os.Remove(path), unix.Flock(fd, unix.LOCK_UN), unix.Close(fd))
	}, nil
}
