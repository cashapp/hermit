package app

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util/debug"
)

type execCmd struct {
	Binary string   `arg:"" help:"Binary symlink to execute."`
	Args   []string `arg:"" help:"Arguments to pass to executable (use -- to separate)." optional:""`
}

func (e *execCmd) Run(l *ui.UI, cache *cache.Cache, sta *state.State, globalState GlobalState, config Config, defaultHTTPClient *http.Client, sourceRewriters []sources.URLRewriter) error {
	envDir, err := hermit.FindEnvDir(e.Binary)
	if err != nil {
		return errors.WithStack(err)
	}
	envInfo, err := hermit.LoadEnvInfo(envDir)
	if err != nil {
		return errors.WithStack(err)
	}

	// Pass config.SHA256Sums because OpenEnv uses the defaults cashapp/hermit; internal builds inject additional SHA256Sums.
	env, err := hermit.OpenEnv(envInfo, sta, cache.GetSource, globalState.Env, defaultHTTPClient, config.SHA256Sums, sourceRewriters...)
	if err != nil {
		return errors.WithStack(err)
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
		return syscall.Exec(self, args, env)
	}

	pkg, binary, err := env.ResolveLink(l, e.Binary)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := pkg.EnsureSupported(); err != nil {
		return errors.Wrapf(err, "execution failed")
	}

	// Run any pre-execution triggers.
	messages, err := env.TriggerForPackage(l, manifest.EventExec, pkg)
	if err != nil {
		return errors.WithStack(err)
	}
	w := l.WriterAt(ui.LevelInfo)
	for _, message := range messages {
		fmt.Fprintln(w, message)
	}
	installed, err := env.ListInstalledReferences()
	if err != nil {
		return errors.WithStack(err)
	}

	// Collect dependencies we might have to install if they are not in the
	// cache
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
