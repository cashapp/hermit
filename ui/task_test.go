package ui

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestConcurrentTaskSizeAndAddNoDeadlock is a regression test for a deadlock
// surfaced when hermit install was parallelised in #558.
//
// Lock-ordering audit:
//
//	Task.Size:           task.lock -> ui.lock        (via swapSize)
//	UI.redrawProgress:   ui.lock   -> task_i.lock    (via liveOperations / op.status)
//
// These orderings are inconsistent. With serial installs only one task is
// actively updating at any moment, so the cycle never closes. Under parallel
// installs many goroutines drive per-package download progress concurrently,
// each calling Task.Size followed by Task.Add for every chunk received
// (cache/http.go), and the cycle closes: goroutine A holds task_A.lock and
// waits on ui.lock; goroutine B holds ui.lock (inside redrawProgress after its
// own Add) and iterates operations waiting on task_A.lock.
//
// The test races Size and Add across many tasks and fails loudly on deadlock
// rather than hanging for the 10-minute Go test default.
func TestConcurrentTaskSizeAndAddNoDeadlock(t *testing.T) {
	ui, _ := NewForTesting()
	const tasks = 8
	const iterations = 200

	var wg sync.WaitGroup
	for i := 0; i < tasks; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task := ui.Progress("task", 1)
			// Mirror cache/http.go: Size() then Add() per chunk.
			for j := 1; j <= iterations; j++ {
				task.Size(j)
				task.Add(1)
			}
			task.Done()
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		t.Fatalf("deadlock: concurrent Task.Size/Task.Add did not complete within 10s\n\ngoroutines:\n%s", buf[:n])
	}
}
