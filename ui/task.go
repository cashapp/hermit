package ui

import (
	"io"
)

// Task encapsulates progress and logging for a single operation.
//
// Operations are not thread safe.
type Task struct {
	loggingMixin

	w        *UI
	started  bool
	progress int
	size     int
}

var _ Logger = &Task{}

// SubTask creates a new unsized subtask.
//
// The size of the operations progress can be configured later.
func (o *Task) SubTask(subtask string) *Task {
	return o.w.operation(o.task, subtask, 0, true)
}

// SubProgress registers and returns a new subtask with progress.
func (o *Task) SubProgress(subtask string, size int) *Task {
	return o.w.operation(o.task, subtask, size, false)
}

// WillLog returns true if "level" will be logged.
func (o *Task) WillLog(level Level) bool {
	return o.w.WillLog(level)
}
func (o *Task) status() (progress int, size int, started bool) {
	o.lock.Lock()
	defer o.lock.Unlock()
	return o.progress, o.size, o.started
}

// Size sets the size of the Task.
func (o *Task) Size(n int) *Task {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.w.swapSize(o.size, n)
	o.size = n
	if o.progress > o.size {
		o.progress = o.size
	}
	return o
}

// Add to progress of the Task.
func (o *Task) Add(n int) {
	if n == 0 {
		return
	}
	o.lock.Lock()
	if o.size == 0 {
		o.lock.Unlock()
		panic("can't increase size of empty operation " + o.label())
	}
	o.started = true
	o.progress += n
	if o.progress >= o.size {
		o.progress = o.size
	}
	if o.progress < 0 {
		o.progress = 0
	}
	o.lock.Unlock()
	o.w.redrawProgress()
}

// ProgressWriter returns a writer that moves the progress bar as it is written to.
//
// The Size() should have previously been set to the maximum number of bytes that will be written.
func (o *Task) ProgressWriter() io.Writer {
	return &progressWriter{o}
}

// Done marks the operation as complete.
func (o *Task) Done() {
	o.Add(o.size)
}

type progressWriter struct {
	b *Task
}

func (p *progressWriter) Write(b []byte) (n int, err error) {
	p.b.Add(len(b))
	return len(b), nil
}

type nopSyncer struct{ io.Writer }

func (n nopSyncer) Sync() error { return nil }

func (n nopSyncer) Fd() uintptr { return 0 }
