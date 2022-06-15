package util

import (
	"context"
	"time"

	"github.com/gofrs/flock"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
)

// FileLock abstracts away the file locking mechanism.
// One FileLock corresponds to a one file on disk.
// This does not support multi-threading. Use only from within one go routine.
type FileLock struct {
	lock          *flock.Flock
	file          string
	lockCount     int
	checkInterval time.Duration
}

// NewLock creates a new file lock.
func NewLock(file string, checkInterval time.Duration) *FileLock {
	return &FileLock{file: file, checkInterval: checkInterval}
}

// Acquire takes the lock. For every Acquire, Release needs to be called later.
// Returns immediately if this process already holds the lock.
func (l *FileLock) Acquire(ctx context.Context, log ui.Logger) error {
	if l.lock == nil {
		lock := flock.New(l.file)
		gotLock, err := lock.TryLock()
		if err != nil {
			return errors.WithStack(err)
		}
		if !gotLock {
			log.Warnf("%s", "Waiting for a lock at "+l.file)
			ticker := time.NewTicker(l.checkInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					gotLock, err := lock.TryLock()
					if err != nil {
						return errors.WithStack(err)
					}
					if gotLock {
						l.lock = lock
						l.lockCount = 1
						return nil
					}
				case <-ctx.Done():
					deadLine, _ := ctx.Deadline()
					return errors.Errorf("timeout while waiting for the lock after %s", time.Until(deadLine))
				}
			}
		}
		l.lock = lock
		l.lockCount = 0
	}
	l.lockCount++
	return nil
}

// Release releases the lock. If there is an error while releasing,
// the error is logged
func (l *FileLock) Release(log ui.Logger) {
	l.lockCount--
	if l.lockCount <= 0 {
		// If the release fails, log an error but allow the execution to continue
		if err := l.lock.Unlock(); err != nil {
			log.Errorf("%s", err.Error())
		}
		l.lock = nil
	}
}
