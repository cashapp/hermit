// Package ui provides the terminal UI formatter for Hermit.
//
// This encapsulates both logging and progress.
//
// Output will be cleared if the higher level Hermit operation is
// successful.
//
// Hermit progress is conveyed via a single progress bar at the bottom of
// its output, ala modern Ubuntu apt progress. The line below the progress
// bar will be the list of concurrent actions being run. The capacity of
// the progress bar will dynamically adjust as new tasks are added.
//
// The progress bar will use partial Unicode blocks:
// https://en.wikipedia.org/wiki/Block_Elements#Character_table
//
// Log output appears above the progress bar.
package ui

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/cashapp/hermit/errors"
)

// SyncWriter is an io.Writer that can be Sync()ed.
type SyncWriter interface {
	io.Writer
	Sync() error
}

// UI controls the display of logs and task progress.
type UI struct {
	*loggingMixin
	tty                *os.File
	width              int
	stdout             SyncWriter
	stderr             SyncWriter
	stdoutIsTTY        bool
	stderrIsTTY        bool
	haveProgress       bool
	size               int64 // Total size of the progress bar. This dynamically changes as new operations are registered.
	operations         []*Task
	state              uint64
	minlevel           Level
	progressBarEnabled bool
}

var _ Logger = &UI{}

// NewForTesting returns a new UI that writes all output to the returned bytes.Buffer.
func NewForTesting() (*UI, *bytes.Buffer) {
	b := &bytes.Buffer{}
	w := nopSyncer{b}
	ui := New(LevelTrace, w, w, true, true)
	return ui, b
}

// New creates a new UI.
func New(level Level, stdout, stderr SyncWriter, stdoutIsTTY, stderrIsTTY bool) *UI {
	w := &UI{
		tty:                os.Stdout,
		stdout:             stdout,
		stdoutIsTTY:        stdoutIsTTY,
		stderr:             stderr,
		stderrIsTTY:        stderrIsTTY,
		minlevel:           level,
		progressBarEnabled: true,
	}
	w.loggingMixin = &loggingMixin{
		logWriter: logWriter{
			level: LevelDebug,
			logf: func(level Level, format string, args ...interface{}) {
				w.logf(level, "", format, args...)
			},
		},
		logf: w.logf,
	}
	w.updateWidth()
	winch := make(chan os.Signal, 1)
	go func() {
		for range winch {
			w.updateWidth()
		}
	}()
	signal.Notify(winch, syscall.SIGWINCH)
	return w
}

// SetLevel sets the UI's minimum log level.
func (w *UI) SetLevel(level Level) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.minlevel = level
}

// SetProgressBarEnabled defines if we want to show the progress bar to the user
func (w *UI) SetProgressBarEnabled(enabled bool) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.progressBarEnabled = enabled
}

// WillLog returns true if "level" will be logged.
func (w *UI) WillLog(level Level) bool {
	w.lock.Lock()
	defer w.lock.Unlock()
	return w.minlevel.Visible(level)
}

// Task creates a new unstarted task.
//
// The resulting Task can be used as a ui.Logger.
//
// Task progress can be modified later.
func (w *UI) Task(task string) *Task {
	return w.operation(task, "", 0, true)
}

// Progress creates a new task with progress indicator of size.
//
// The resulting Task can be used as a ui.Logger.
func (w *UI) Progress(task string, size int) *Task {
	return w.operation(task, "", size, false)
}

// If "lazy" is true, the returned Task will not contribute to the progress indicator or task list.
func (w *UI) operation(task, subtask string, size int, lazy bool) *Task {
	w.lock.Lock()
	w.size += int64(size)
	op := &Task{
		loggingMixin: loggingMixin{
			logWriter: logWriter{
				level: w.level,
				logf: func(level Level, format string, args ...interface{}) {
					w.logf(level, "", format, args...)
				},
			},
			task:    task,
			subtask: subtask,
			logf:    w.logf,
		},
		size:    size,
		started: !lazy,
		w:       w,
	}
	w.operations = append(w.operations, op)
	w.lock.Unlock()
	w.redrawProgress()
	return op
}

// Clear the progress indicator.
func (w *UI) Clear() {
	w.lock.Lock()
	w.clearProgress()
	_ = w.stdout.Sync()
	w.lock.Unlock()
}

// Change the size of a Task.
func (w *UI) swapSize(oldSize, newSize int) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.size += int64(newSize - oldSize)
}

func (w *UI) logf(level Level, label string, format string, args ...interface{}) {
	if !w.minlevel.Visible(level) {
		return
	}
	w.lock.Lock()
	defer w.lock.Unlock()

	// Whether to ANSI format the output.
	ansi := w.stdoutIsTTY && level < LevelWarn || w.stderrIsTTY && level >= LevelWarn

	// Format the message.
	var msg string
	if ansi {
		msg += "\033[1m" + levelColor[level]
	}
	msg += level.String() + ":"
	if label != "" {
		msg += label + ":"
	}
	msg += " "
	if ansi {
		msg += "\033[0m" + levelColor[level]
		msg += fmt.Sprintf(format, args...)
		msg += "\033[0m\033[0K"
	} else {
		msg += fmt.Sprintf(format, args...)
	}
	w.clearProgress()
	switch {
	case w.stdoutIsTTY && level < LevelWarn:
		fmt.Fprintf(w.stdout, "%s\n", msg)

	case level >= LevelWarn:
		fmt.Fprintf(w.stderr, "%s\n", msg)
	}
	if level == LevelFatal {
		defer func() {
			_ = w.stderr.Sync()
		}()
	} else {
		w.writeProgress(w.width)
	}
}

func (w *UI) redrawProgress() {
	w.lock.Lock()
	defer w.lock.Unlock()
	if state := w.operationState(); state == w.state {
		return
	} else { // nolint
		w.state = state
	}
	w.clearProgress()
	w.writeProgress(w.width)
}

// Internal only, does not acquire lock.
func (w *UI) clearProgress() {
	if !w.progressBarEnabled || !w.stdoutIsTTY || !w.haveProgress {
		return
	}
	// Clear previous progress indicator.
	for range 2 {
		fmt.Fprintf(w.stdout, "\033[0A\033[2K\r") // Move up and clear line
	}
}

// Internal only, does not acquire lock.
func (w *UI) writeProgress(width int) {
	if !w.progressBarEnabled {
		return
	}
	liveOperations := w.liveOperations()
	w.haveProgress = len(liveOperations) > 0
	if !w.haveProgress || !w.stdoutIsTTY {
		return
	}
	// Collect progress status.
	progress := 0
	size := 0
	complete := 1
	for _, op := range liveOperations {
		opprogress, opsize, _ := op.status()
		if opprogress >= opsize {
			complete++
		}
		progress += opprogress
		size += opsize
	}
	// We want to have the count of tasks as 1/2 rather than 0/2; this avoids 3/2
	if complete > len(liveOperations) {
		complete = len(liveOperations)
	}
	// Format progress bar.
	percent := float64(progress) / float64(size)
	barsn := len(theme.bars)
	columns := int(float64(width-15) * float64(barsn) * percent)
	nofm := fmt.Sprintf("%d/%d", complete, len(liveOperations))
	percentstr := fmt.Sprintf("%.1f%%", percent*100)
	spaces := max(width - columns/barsn - 15, 0)
	fmt.Fprintf(w.stdout, "%s%s%s %-7s%6s\n", strings.Repeat(theme.fill, max(columns/barsn, 0)), theme.bars[columns%barsn], strings.Repeat(theme.blank, spaces), nofm, percentstr)
	// Write operations bar.
	for _, op := range liveOperations {
		opprogress, opsize, _ := op.status()
		if opprogress < opsize {
			fmt.Fprintf(w.stdout, "\033[0m%s ", op.label())
		}
	}
	fmt.Fprintf(w.stdout, "\033[0m\033[0K\n")
	_ = w.stdout.Sync()
}

// Internal only, does not acquire lock.
func (w *UI) liveOperations() []*Task {
	liveOperations := make([]*Task, 0, len(w.operations))
	for _, op := range w.operations {
		_, _, started := op.status()
		if started {
			liveOperations = append(liveOperations, op)
		}
	}
	return liveOperations
}

// Internal only, does not acquire lock.
func (w *UI) operationState() uint64 {
	h := fnv.New64a()
	for _, op := range w.liveOperations() {
		progress, size, started := op.status()
		fmt.Fprintf(h, "%v:%v:%v\n", progress, size, started)
	}
	return h.Sum64()
}

func (w *UI) updateWidth() {
	w.lock.Lock()
	var err error
	w.width, _, err = term.GetSize(int(w.tty.Fd()))
	if err != nil || w.width < 20 { // Assume it's borked.
		w.width = 80
	}
	w.lock.Unlock()
}

// Printf prints directly to the stdout without log formatting
func (w *UI) Printf(format string, args ...interface{}) {
	w.lock.Lock()
	fmt.Fprintf(w.stdout, format, args...)
	w.lock.Unlock()
}

// Confirmation from the user with y/N options to proceed
func (w *UI) Confirmation(message string, args ...interface{}) (bool, error) {
	w.lock.Lock()
	defer w.lock.Unlock()

	fmt.Fprintf(w.stdout, "hermit: "+message+" ", args...)
	if err := w.stdout.Sync(); err != nil {
		return false, errors.WithStack(err)
	}
	s := ""
	if _, err := fmt.Scan(&s); err != nil {
		return false, errors.WithStack(err)
	}

	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	return s == "y" || s == "yes", nil
}

// Sync flushes IO to stdout and stderr.
func (w *UI) Sync() error {
	w.lock.Lock()
	defer w.lock.Unlock()
	return errors.Join(w.stdout.Sync(), w.stderr.Sync())
}
