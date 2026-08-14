package util

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kballard/go-shellquote"

	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/internal/redact"
	"github.com/cashapp/hermit/ui"
)

// SystemCommand constructs a command for an external tool used internally by
// Hermit. The executable is resolved from PATH with the active Hermit
// environment's changes reverted, and that same PATH is inherited by its
// child processes.
func SystemCommand(args ...string) (*exec.Cmd, error) {
	if len(args) == 0 {
		return nil, errors.New("missing system command")
	}
	name := args[0]
	if filepath.Base(name) != name {
		return nil, errors.Errorf("system command must be a bare name: %q", name)
	}
	environ, err := systemEnviron()
	if err != nil {
		return nil, err
	}
	var path string
	for _, dir := range filepath.SplitList(environ["PATH"]) {
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
	cmd.Env = environ.System()
	return cmd, nil
}

// systemEnviron returns the current environment with PATH restored to its
// state before Hermit activation. All other environment variables are left
// unchanged.
func systemEnviron() (envars.Envars, error) {
	environ := envars.Parse(os.Environ())
	data := os.Getenv("HERMIT_ENV_OPS")
	if data == "" {
		return environ, nil
	}
	ops, err := envars.UnmarshalOps([]byte(data))
	if err != nil {
		return nil, errors.Wrap(err, "failed to restore PATH before Hermit activation")
	}
	reverted := environ.Revert(os.Getenv("HERMIT_ENV"), ops).Combined()
	path, ok := reverted["PATH"]
	if !ok {
		delete(environ, "PATH")
	} else {
		environ["PATH"] = path
	}
	return environ, nil
}

// Run a command, outputting to stdout and stderr.
func Run(log *ui.Task, args ...string) error {
	return RunInDir(log, "", args...)
}

// RunSystem runs an external tool used internally by Hermit.
func RunSystem(log *ui.Task, args ...string) error {
	return RunSystemInDir(log, "", args...)
}

// Capture runs a command, returning combined stdout and stderr.
func Capture(log ui.Logger, args ...string) ([]byte, error) {
	log.Debugf("%s", redact.Credentials(shellquote.Join(args...)))
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec,noctx
	return captureOutput(log, cmd)
}

// CaptureSystem runs an external tool used internally by Hermit and returns its output.
func CaptureSystem(log ui.Logger, args ...string) ([]byte, error) {
	return CaptureSystemInDir(log, "", args...)
}

// CaptureSystemInDir runs an external tool used internally by Hermit in the given dir
// and returns its output.
func CaptureSystemInDir(log ui.Logger, dir string, args ...string) ([]byte, error) {
	log.Debugf("%s", redact.Credentials(shellquote.Join(args...)))
	cmd, err := SystemCommand(args...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	cmd.Dir = dir
	return captureOutput(log, cmd)
}

// CaptureInDir runs a command in the given dir, returning combined stdout and stderr.
func CaptureInDir(log ui.Logger, dir string, args ...string) ([]byte, error) {
	log.Debugf("%s", redact.Credentials(shellquote.Join(args...)))
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec,noctx
	cmd.Dir = dir
	return captureOutput(log, cmd)
}

func captureOutput(log ui.Logger, cmd *exec.Cmd) ([]byte, error) {
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, errors.Wrapf(err, "%s: %s",
			redact.Credentials(shellquote.Join(cmd.Args...)),
			redact.Credentials(strings.TrimSpace(string(out))))
	}
	_, _ = log.Write([]byte(redact.Credentials(string(out))))
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
			log.Errorf("%s", redact.Credentials(out.String()))
		}
		return errors.Wrapf(err, "%s failed", redact.Credentials(shellquote.Join(args...)))
	}
	return nil
}

// RunSystemInDir runs an external tool used internally by Hermit in the given dir.
func RunSystemInDir(log *ui.Task, dir string, args ...string) error {
	return runSystemInDir(log, dir, args, args)
}

// RunSystemInDirWithSource runs a command whose arguments contain a source URI.
// The raw source is used only for execution; logging and errors use String().
func RunSystemInDirWithSource(
	log *ui.Task,
	dir string,
	argsBeforeSource []string,
	source interface {
		Get() string
		String() string
	},
	argsAfterSource ...string,
) error {
	rawArgs := make([]string, 0, len(argsBeforeSource)+1+len(argsAfterSource))
	rawArgs = append(rawArgs, argsBeforeSource...)
	rawArgs = append(rawArgs, source.Get())
	rawArgs = append(rawArgs, argsAfterSource...)

	displayArgs := make([]string, 0, len(rawArgs))
	displayArgs = append(displayArgs, argsBeforeSource...)
	displayArgs = append(displayArgs, source.String())
	displayArgs = append(displayArgs, argsAfterSource...)
	return runSystemInDir(log, dir, rawArgs, displayArgs)
}

func runSystemInDir(log *ui.Task, dir string, args, displayArgs []string) error {
	log = log.SubTask("exec")
	display := redact.Credentials(shellquote.Join(displayArgs...))
	log.Debugf("%s", display)
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
			log.Errorf("%s", redact.Credentials(b.String()))
		}
		return errors.Wrapf(err, "%s failed", display)
	}
	return nil
}

// Command constructs a new exec.Cmd with logging configured.
//
// Returns the command, and a *bytes.Buffer containing the combined stdout and stderr
// of the execution
func Command(log *ui.Task, args ...string) (*exec.Cmd, *bytes.Buffer) {
	log = log.SubTask("exec")
	log.Debugf("%s", redact.Credentials(shellquote.Join(args...)))
	b := &bytes.Buffer{}
	w := io.MultiWriter(b, log)
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec,noctx
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd, b
}
