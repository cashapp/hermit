package util

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kballard/go-shellquote"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
)

// CommandRunner abstracts how we run command in a given directory
type CommandRunner interface {
	// RunInDir runs a command in the given directory.
	RunInDir(log *ui.Task, dir string, args ...string) error
}

// RealCommandRunner actually calls command
type RealCommandRunner struct{}

func (g *RealCommandRunner) RunInDir(task *ui.Task, dir string, commands ...string) error {
	return errors.WithStack(RunSystemInDir(task, dir, commands...))
}

// systemPath is the trusted path used for Hermit's own helper processes. In
// particular, it excludes Hermit environment bin directories, which may be
// controlled by the repository being operated on.
func systemPath() string {
	return "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
}

// SystemCommand constructs a command for one of Hermit's own system helpers.
// The executable is resolved only from the system path, and the same path is
// inherited by the helper's child processes.
func SystemCommand(args ...string) (*exec.Cmd, error) {
	if len(args) == 0 {
		return nil, errors.New("missing system command")
	}
	name := args[0]
	if filepath.Base(name) != name {
		return nil, errors.Errorf("system command must be a bare name: %q", name)
	}
	var path string
	for _, dir := range filepath.SplitList(systemPath()) {
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() && info.Mode().Perm()&0111 != 0 {
			path = candidate
			break
		}
	}
	if path == "" {
		return nil, &exec.Error{Name: name, Err: exec.ErrNotFound}
	}
	cmd := exec.Command(path, args[1:]...) //nolint:noctx
	cmd.Env = systemEnviron()
	return cmd, nil
}

func systemEnviron() []string {
	environ := os.Environ()
	out := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		key, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "PATH") {
			continue
		}
		out = append(out, entry)
	}
	return append(out, "PATH="+systemPath())
}

// Run a command, outputting to stdout and stderr.
func Run(log *ui.Task, args ...string) error {
	return RunInDir(log, "", args...)
}

// RunSystem runs one of Hermit's own system helpers.
func RunSystem(log *ui.Task, args ...string) error {
	return RunSystemInDir(log, "", args...)
}

// Capture runs a command, returning combined stdout and stderr.
func Capture(log ui.Logger, args ...string) ([]byte, error) {
	log.Debugf("%s", shellquote.Join(args...))
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec,noctx
	return captureOutput(log, cmd)
}

// CaptureSystem runs one of Hermit's own system helpers and returns its output.
func CaptureSystem(log ui.Logger, args ...string) ([]byte, error) {
	return CaptureSystemInDir(log, "", args...)
}

// CaptureSystemInDir runs one of Hermit's own system helpers in the given dir
// and returns its output.
func CaptureSystemInDir(log ui.Logger, dir string, args ...string) ([]byte, error) {
	log.Debugf("%s", shellquote.Join(args...))
	cmd, err := SystemCommand(args...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	cmd.Dir = dir
	return captureOutput(log, cmd)
}

// CaptureInDir runs a command in the given dir, returning combined stdout and stderr.
func CaptureInDir(log ui.Logger, dir string, args ...string) ([]byte, error) {
	log.Debugf("%s", shellquote.Join(args...))
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec,noctx
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

// RunSystemInDir runs one of Hermit's own system helpers in the given dir.
func RunSystemInDir(log *ui.Task, dir string, args ...string) error {
	log = log.SubTask("exec")
	log.Debugf("%s", shellquote.Join(args...))
	b := &bytes.Buffer{}
	w := io.MultiWriter(b, log)
	cmd, err := SystemCommand(args...)
	if err != nil {
		return errors.WithStack(err)
	}
	cmd.Dir = dir
	cmd.Stdout = w
	cmd.Stderr = w
	if err = cmd.Run(); err != nil {
		if !log.WillLog(ui.LevelDebug) {
			log.Errorf("%s", b.String())
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
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec,noctx
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd, b
}
