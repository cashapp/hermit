package app

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util/debug"
)

type execCmd struct {
	Binary string   `arg:"" help:"Binary symlink to execute."`
	Args   []string `arg:"" help:"Arguments to pass to executable (use -- to separate)." optional:""`
}

func (e *execCmd) Run(l *ui.UI, cache *cache.Cache, sta *state.State, env *hermit.Env, globalState GlobalState, config Config, defaultHTTPClient *http.Client) error {
	envDir, err := hermit.EnvDirFromProxyLink(e.Binary)
	if err != nil {
		return errors.WithStack(err)
	}
	// If we're running a binary from a different environment, activate it first.
	activeEnv := os.Getenv("ACTIVE_HERMIT")
	if env == nil || (activeEnv != "" && envDir != "" && activeEnv != envDir) {
		env, err = hermit.OpenEnv(envDir, sta, cache.GetSource, globalState.Env, defaultHTTPClient, nil)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	args := []string{e.Binary}
	args = append(args, e.Args...)

	self, err := os.Executable()
	if err != nil {
		return errors.WithStack(err)
	}

	// Upgrade hermit if necessary
	pkgRef := filepath.Base(filepath.Dir(self))
	if strings.HasPrefix(pkgRef, "hermit@") {
		err := updateHermit(l, env, pkgRef, false)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// Special-case executing Hermit itself.
	if filepath.Base(e.Binary) == "hermit" {
		env := os.Environ()
		env = append(env, "HERMIT_ENV="+envDir)
		cmd := &exec.Cmd{
			Path:        self,
			Args:        args,
			Env:         env,
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
			Stdin:       os.Stdin,
			SysProcAttr: &syscall.SysProcAttr{Setpgid: true},
		}
		err = cmd.Run()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		// return syscall.Exec(self, args, env)
		return errors.Wrapf(err, "failed to execute %q", e.Binary)
	}

	pkg, binary, err := env.ResolveLink(l, e.Binary)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := pkg.EnsureSupported(); err != nil {
		return errors.Wrapf(err, "execution failed")
	}
	installed, err := env.ListInstalledReferences()
	if err != nil {
		return errors.WithStack(err)
	}

	// Collect dependencies we might have to install
	// if they are not in the cache
	deps := map[string]*manifest.Package{}
	err = env.ResolveWithDeps(l, installed, manifest.ExactSelector(pkg.Reference), deps)
	if err != nil {
		return errors.WithStack(err)
	}

	return env.Exec(l, pkg, binary, args, deps)
}

func updateHermit(l *ui.UI, env *hermit.Env, pkgRef string, force bool) error {
	l.Tracef("Checking if %s needs to be updated", pkgRef)
	pkg, err := env.Resolve(l, manifest.ExactSelector(manifest.ParseReference(pkgRef)), false)
	if err != nil {
		return errors.WithStack(err)
	}
	// Mark Hermit updated if this is a new installation to prevent immediate upgrade checks
	if force {
		pkg.UpdatedAt = time.Time{}
	} else if pkg.UpdatedAt.IsZero() {
		pkg.UpdatedAt = time.Now()
	}
	err = env.UpdateUsage(pkg)
	if err != nil {
		return errors.WithStack(err)
	}

	if debug.Flags.AlwaysCheckSelf {
		// set the update time to 0 to force an update check
		pkg.UpdatedAt = time.Time{}
	}
	return errors.WithStack(env.EnsureChannelIsUpToDate(l, pkg))
}
