package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/hcl"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/platform"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

type addDigestsCmd struct {
	ManifestFiles []string `arg:"" help:"List of files that need to be updated with digests"`
}

func (e *addDigestsCmd) Help() string {
	return `
	This command will go through each manifest file in input and add missing digest values.
	It will use the sha256sums attribute in the manifest files to add digests.
	Note: It might download packages that are not in the local cache. So it might take some time.
	`
}
func (e *addDigestsCmd) Run(l *ui.UI, state *state.State) error {
	for _, f := range e.ManifestFiles {
		absolutePath, err := filepath.Abs(f)
		if err != nil {
			return errors.WithStack(err)
		}
		dir := filepath.Dir(absolutePath)
		packageName := strings.Replace(f, ".hcl", "", 1)
		task := l.Task(packageName)

		localmanifest := manifest.LoadManifestFile(os.DirFS(dir), packageName, f)
		if localmanifest == nil {
			return errors.New("Cannot load manifest: " + f)
		}
		if localmanifest.Manifest.SHA256Sums == nil {
			localmanifest.Manifest.SHA256Sums = make(map[string]string)
		}
		err = populateDigests(l, state, localmanifest)
		if err != nil {
			return errors.WithStack(err)
		}
		var value []byte
		value, err = hcl.Marshal(localmanifest.Manifest)
		if err != nil {
			return errors.WithStack(err)
		}
		err = os.WriteFile(f, value, os.ModePerm)
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

func populateDigests(l *ui.UI, state *state.State, localManifest *manifest.AnnotatedManifest) error {
	l.Infof("Working on %s package", localManifest.Name)
	for _, mc := range localManifest.Manifest.Versions {
		ref := manifest.ParseReference(localManifest.Name + "-" + mc.Version[0])
		for _, p := range platform.Core {

			pkg, err := manifest.NewPackage(localManifest, p, ref)
			if err != nil {
				l.Tracef("Continuing with the next platform tuple.  Current %s: %s", p.OS, p.Arch)
				continue
			}
			// optimize for an already present value.
			// Trust model here is that an existing value is correct which is the assumption anyway.
			if _, ok := localManifest.Manifest.SHA256Sums[pkg.Source]; ok {
				l.Tracef("Skipping shasum for %s as it's already present", pkg.Source)
				continue
			}
			var digest string
			digest, err = getDigest(l, state, pkg, ref)
			if err != nil {
				return errors.WithStack(err)
			}
			localManifest.Manifest.SHA256Sums[pkg.Source] = digest
		}
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
