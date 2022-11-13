package util

import (
	"bytes"
	"io"
	"os/exec"
	"strings"

	"github.com/kballard/go-shellquote"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
)

// CommandRunner abstracts how we run command in a given directory
type CommandRunner interface {
	// RunInDir runs a command in the given directory.
	RunInDir(log *ui.Task, dir string, args ...string) error
	// CaptureInDir runs a command in the given dir, returning combined stdout and stderr.
	CaptureInDir(log ui.Logger, dir string, args ...string) ([]byte, error)
}

// RealCommandRunner actually calls command
type RealCommandRunner struct{}

var _ CommandRunner = &RealCommandRunner{}

// CaptureInDir implements CommandRunner
func (*RealCommandRunner) CaptureInDir(log ui.Logger, dir string, args ...string) ([]byte, error) {
	data, err := CaptureInDir(log, dir, args...)
	return data, errors.WithStack(err)
}

func (g *RealCommandRunner) RunInDir(task *ui.Task, dir string, commands ...string) error { // nolint: golint
	return errors.WithStack(RunInDir(task, dir, commands...))
}

// Run a command, outputting to stdout and stderr.
func Run(log *ui.Task, args ...string) error {
	return RunInDir(log, "", args...)
}

// Capture runs a command, returning combined stdout and stderr.
func Capture(log ui.Logger, args ...string) ([]byte, error) {
	log.Debugf("%s", shellquote.Join(args...))
	cmd := exec.Command(args[0], args[1:]...)
	return captureOutput(log, cmd)
}

// CaptureInDir runs a command in the given dir, returning combined stdout and stderr.
func CaptureInDir(log ui.Logger, dir string, args ...string) ([]byte, error) {
	log.Debugf("%s", shellquote.Join(args...))
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	return captureOutput(log, cmd)
}

func captureOutput(log ui.Logger, cmd *exec.Cmd) ([]byte, error) {
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, errors.Wrapf(err, "%s: %s", shellquote.Join(cmd.Args...), strings.TrimSpace(string(out)))
	}
	_, _ = log.Write(out)
	return out, nil
}

// RunInDir runs a command in the given directory.
func RunInDir(log *ui.Task, dir string, args ...string) error {
	cmd, out := Command(log, args...)
	cmd.Dir = dir
	err := cmd.Run()
	if err != nil {
		// log.Write() goes to debug, so only dump the logs at error if we haven't already.
		if !log.WillLog(ui.LevelDebug) {
			log.Errorf("%s", out.String())
		}
		return errors.Wrapf(err, "%s failed", shellquote.Join(args...))
	}
	return nil
}

// Command constructs a new exec.Cmd with logging configured.
//
// Returns the command, and a *bytes.Buffer containing the combined stdout and stderr
// of the execution
func Command(log *ui.Task, args ...string) (*exec.Cmd, *bytes.Buffer) {
	log = log.SubTask("exec")
	log.Debugf("%s", shellquote.Join(args...))
	b := &bytes.Buffer{}
	w := io.MultiWriter(b, log)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd, b
}
