package shell

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"

	"github.com/mitchellh/go-ps"
	"github.com/pkg/errors"

	"github.com/willdonnelly/passwd"

	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

// ActivationConfig for shells.
type ActivationConfig struct {
	Root   string
	Prompt string
	Env    envars.Envars
}

// Shell abstracts shell specific functionality
type Shell interface {
	Name() string
	// ActivationHooksInstallation returns the path and shell fragment for injecting activation code to the shell initialisation.
	ActivationHooksInstallation() (path, script string, err error)
	// ActivationHooksCode returns the shell fragment for activation/deactivation hooks
	ActivationHooksCode() (script string, err error)
	// ActivationScript for this shell.
	ActivationScript(w io.Writer, config ActivationConfig) error
	// ApplyEnvars writes the shell fragment required to apply the given envars.
	//
	// Envars with empty values should be deleted.
	ApplyEnvars(w io.Writer, env envars.Envars) error
	// DeactivationScript for this shell.
	DeactivationScript(w io.Writer) error
}

var (
	shells = map[string]Shell{
		"zsh":  &Zsh{},
		"bash": &Bash{},
	}
)

// InstallHooks for the given Shell.
func InstallHooks(l *ui.UI, shell Shell) error {
	if shell == nil {
		return nil
	}
	fileName, script, err := shell.ActivationHooksInstallation()
	if err != nil {
		return errors.WithStack(err)
	}
	patcher := util.NewFilePatcher(hookStartMarker, hookEndMarker)
	changed, err := patcher.Patch(fileName, script)
	if err != nil {
		return errors.WithStack(err)
	}
	if changed {
		l.Infof("Hermit hooks updated to %s", fileName)
	} else {
		l.Infof("Hermit hooks were already up-to-date in %s", fileName)
	}
	return nil
}

// PrintHooks for the given shell.
//
// "sha256sums" is the union of all known SHA256 sums for per-environment scripts.
func PrintHooks(shell Shell, sha256sums []string) error {
	if shell == nil {
		return nil
	}
	code, err := shell.ActivationHooksCode()
	if err != nil {
		return errors.WithStack(err)
	}
	fmt.Fprint(os.Stdout, code)
	return nil
}

// Detect the user's shell.
func Detect() (Shell, error) {
	// First look for shell in parent processes.
	pid := os.Getppid()
	for {
		process, err := ps.FindProcess(pid)
		if err != nil || process == nil {
			break
		}
		name := filepath.Base(process.Executable())
		shell, ok := shells[name]
		if ok {
			return shell, nil
		}
		pid = process.PPid()
		if pid == 0 {
			break
		}
	}

	// Next, try to pull the shell from the user's password entry.
	u, err := user.Current()
	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve current user")
	}
	pw, err := passwd.Parse()
	if err != nil {
		return nil, errors.Wrap(err, "couldn't locate/parse /etc/passwd database")
	}
	entry, ok := pw[u.Name]
	if !ok {
		return nil, errors.Errorf("/etc/passwd file entry for %q is missing", u.Name)
	}
	if entry.Shell == "" {
		return nil, errors.Errorf("/etc/passwd file entry for %q does not contain a shell field", u.Name)
	}
	shell, ok := shells[filepath.Base(entry.Shell)]
	if ok {
		return shell, nil
	}
	return nil, errors.Errorf("unsupported shell %q :(", entry.Shell)
}

// Changes encapsulates changes that need to be applied to the active environment after a change
type Changes struct {
	Env    envars.Envars
	Add    envars.Ops
	Remove envars.Ops
}

// NewChanges returns a new empty change set
func NewChanges(env envars.Envars) *Changes {
	return &Changes{Env: env}
}

// Merge combines two change sets into one
func (c Changes) Merge(o *Changes) *Changes {
	if o != nil {
		c.Add = append(c.Add, o.Add...)
		c.Remove = append(c.Remove, o.Remove...)
	}
	return &c
}

// ActivateHermit prints out the hermit activation script for the given shell.
func ActivateHermit(w io.Writer, shell Shell, config ActivationConfig) error {
	if err := shell.ApplyEnvars(w, config.Env); err != nil {
		return errors.WithStack(err)
	}
	if err := shell.ActivationScript(w, config); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// DeactivateHermit prints out the hermit deactivation script for the given shell.
func DeactivateHermit(w io.Writer, shell Shell, env envars.Envars) error {
	if err := shell.DeactivationScript(w); err != nil {
		return errors.WithStack(err)
	}
	if err := shell.ApplyEnvars(w, env); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
