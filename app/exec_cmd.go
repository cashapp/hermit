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

func (e *execCmd) Run(
	l *ui.UI,
	cli cliInterface,
	cache *cache.Cache,
	sta *state.State,
	globalState GlobalState,
	config Config,
	defaultHTTPClient *http.Client,
	sourceRewriters []sources.URLRewriter,
) error {
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
	err = maybeUpdateHermit(l, sta, cli.getSelfUpdate(), false)
	if err != nil {
		return errors.WithStack(err)
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

// Upgrade hermit if necessary. We do this to ensure we can read the latest versions of manifests, otherwise they
// might have configuration that is unavailable in older versions.
//
// If "selfUpdate" is false this will be a no-op under all circumstances.
func maybeUpdateHermit(l *ui.UI, state *state.State, selfUpdate bool, force bool) error {
	if !selfUpdate {
		return nil
	}
	self, err := os.Executable()
	if err != nil {
		return errors.WithStack(err)
	}

	pkgRef := filepath.Base(filepath.Dir(self))
	if !strings.HasPrefix(pkgRef, "hermit@") {
		return nil
	}
	return updateHermitChannel(l, state, manifest.ParseReference(pkgRef), force)
}

// updateHermitChannel upgrades the given Hermit channel package if its update interval has elapsed.
// The last check time is the metadata file's mtime, so writing state on every run would stop checks from ever firing.
func updateHermitChannel(l *ui.UI, state *state.State, ref manifest.Reference, force bool) error {
	l.Tracef("Checking if %s needs to be updated", ref)
	pkg, err := state.Resolve(l, manifest.ExactSelector(ref))
	if err != nil {
		return errors.WithStack(err)
	}
	state.ReadPackageState(pkg)
	switch {
	case force || debug.Flags.AlwaysCheckSelf:
		pkg.UpdatedAt = time.Time{}
	case pkg.UpdatedAt.IsZero():
		// Record a new installation as just checked to prevent an immediate upgrade check.
		pkg.UpdatedAt = time.Now()
		return errors.WithStack(state.WritePackageState(pkg))
	}
	return errors.WithStack(state.EnsureChannelIsUpToDate(l, pkg))
}
