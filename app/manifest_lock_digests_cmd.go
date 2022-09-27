package app

import (
	"os"
	"path/filepath"

	"github.com/cashapp/hermit/platform"

	"github.com/alecthomas/hcl"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/manifest"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type lockDigestsCmd struct {
}

func (e *lockDigestsCmd) Help() string {
	return `
	This command will build a list of installed packages and their checksums(SHA256) values.
	
	That file can be manually reviewed by the project owner and checked in to the repo.

	On subsequent uses of the hermit environment, the installed dependencies will have a reference checksum

	value for hermit to validate against.
	`
}
func (e *lockDigestsCmd) Run(l *ui.UI, env *hermit.Env, cache *cache.Cache, state *state.State) error {
	//time.Sleep(time.Second * 7)
	installed, err := env.ListInstalledReferences()
	if err != nil {
		return errors.WithStack(err)
	}
	if len(installed) == 0 {
		l.Infof("There are no packages installed. Nothing to do here.")
		return nil
	}
	for _, ref := range installed {
		task := l.Task(ref.String())
		pkg, err := env.Resolve(l, manifest.ExactSelector(ref), false)
		if err != nil {
			return errors.WithStack(err)
		}
		//infer the manifest file
		res, err := env.GetResolver(l)
		if err != nil {
			return errors.WithStack(err)
		}
		localmanifest, err := res.ResolveManifest(l, manifest.ExactSelector(ref))
		if err != nil {
			return errors.WithStack(err)
		}
		// parse through all the version blocks and try to populate the hashes
		if localmanifest.Manifest.SHA256Sums == nil {
			localmanifest.Manifest.SHA256Sums = make(map[string]string)
		}
		l.Infof("Working on %s package", ref.Name)
		for _, mc := range localmanifest.Manifest.Versions {
			for _, p := range platform.Core {
				ref = manifest.ParseReference(ref.Name + "-" + mc.Version[0])
				config := manifest.Config{}
				if p.OS == "Darwin" {
					config = manifest.DarwinConfig(res.GetConfig(), p.Arch)
				} else {
					config = manifest.LinuxConfig(res.GetConfig())
				}
				pkg, err := res.ResolveWithConfig(l, manifest.ExactSelector(ref), config)
				if err != nil {
					l.Debugf("Continuing with the next platform tuple.  Current %s: %s", p.OS, p.Arch)
					continue
				}
				// optimize for an already present value.
				// Trust model here is that an existing value is correct which is the assumption anyway.
				if _, ok := localmanifest.Manifest.SHA256Sums[pkg.Source]; ok {
					l.Debugf("Skipping shasum for %s as it's already present", pkg.Source)
				}
				checksum, err := getSHASum(l, state, pkg, &ref)
				if err != nil {
					return errors.WithStack(err)
				}
				localmanifest.Manifest.SHA256Sums[pkg.Source] = checksum
			}
		}

		value, _ := hcl.Marshal(localmanifest.Manifest)

		//Now just write down fresh manifest files in a subdirectory in the `bin`.
		err = os.MkdirAll(filepath.Join(env.BinDir(), "hermit.lock"), os.ModePerm)
		if err != nil && os.IsExist(err) {
			l.Errorf("Could not create directory %v", err)
			return errors.WithStack(err)
		}
		err = os.WriteFile(filepath.Join(env.BinDir(), "hermit.lock", pkg.Reference.Name+".hcl"), value, os.ModePerm)
		if err != nil {
			l.Errorf("Could not write the manifest file %s", pkg.Reference.Name+".hcl")
			return errors.WithStack(err)
		}

		pkg.LogWarnings(l)
		task.Done()
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}
func getSHASum(l *ui.UI, state *state.State, pkg *manifest.Package, ref *manifest.Reference) (string, error) {
	task := l.Task(ref.String())

	checksum, err := state.CacheAndDontUnpack(task, pkg)
	if err != nil {
		return "", errors.WithStack(err)
	}
	task.Done()
	return checksum, nil

}
