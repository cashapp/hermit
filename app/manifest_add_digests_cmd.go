package app

import (
	"github.com/alecthomas/hcl"
	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/platform"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
	"os"
	"path/filepath"
	"strings"
)

type AddDigestsCmd struct {
	ManifestFile []string `arg:"" help:"Directory containing manifest files that need to be updated"`
}

func (e *AddDigestsCmd) Help() string {
	return `
	This command will build a list of installed packages and their digests(SHA256) values.
	
	That file can be manually reviewed by the project owner and checked in to the repo.

	On subsequent uses of the hermit environment, the installed dependencies will have a reference digest

	value for hermit to validate against.
	`
}
func (e *AddDigestsCmd) Run(l *ui.UI, env *hermit.Env, cache *cache.Cache, state *state.State) error {
	for _, f := range e.ManifestFile {
		absolutePath, er := filepath.Abs(f)
		if er != nil {
			return errors.WithStack(er)
		}
		baseName := f
		dir := filepath.Dir(absolutePath)
		packageName := strings.Replace(f, ".hcl", "", 1)
		task := l.Task(packageName)

		localmanifest := manifest.LoadManifestFile(os.DirFS(dir), packageName, baseName)
		if localmanifest == nil {
			return errors.New("Cannot load manifest")
		}
		if localmanifest.Manifest.SHA256Sums == nil {
			localmanifest.Manifest.SHA256Sums = make(map[string]string)
		}
		l.Infof("Working on %s package", localmanifest.Name)
		for _, mc := range localmanifest.Manifest.Versions {
			ref := manifest.ParseReference(localmanifest.Name + "-" + mc.Version[0])
			for _, p := range platform.Core {

				pkg, err := manifest.NewPackage(localmanifest, p, ref)
				if err != nil {
					l.Debugf("Continuing with the next platform tuple.  Current %s: %s", p.OS, p.Arch)
					continue
				}
				// optimize for an already present value.
				// Trust model here is that an existing value is correct which is the assumption anyway.
				if _, ok := localmanifest.Manifest.SHA256Sums[pkg.Source]; ok {
					l.Debugf("Skipping shasum for %s as it's already present", pkg.Source)
					continue
				}
				var digest string
				digest, err = getDigest(l, state, pkg, ref)
				if err != nil {
					return errors.WithStack(err)
				}
				localmanifest.Manifest.SHA256Sums[pkg.Source] = digest
			}
		}

		value, _ := hcl.Marshal(localmanifest.Manifest)
		err := os.WriteFile(f, value, os.ModePerm)
		if err != nil {
			l.Errorf("Could not write the manifest file %s", f)
			return errors.WithStack(err)
		}

		task.Done()
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}
	return nil
}

func getDigest(l *ui.UI, state *state.State, pkg *manifest.Package, ref manifest.Reference) (string, error) {
	task := l.Task(ref.String())

	digest, err := state.CacheAndDigest(task, pkg)
	if err != nil {
		return "", errors.WithStack(err)
	}
	task.Done()
	return digest, nil

}
