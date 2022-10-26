package manifest

import (
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/platform"
	pstate "github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

// PopulateDigests Add missing digests to the manifest file.
func PopulateDigests(l *ui.UI, state *pstate.State, localManifest *AnnotatedManifest) error {
	l.Infof("Working on %s package", localManifest.Name)
	for _, mc := range localManifest.Manifest.Versions {
		ref := ParseReference(localManifest.Name + "-" + mc.Version[0])
		for _, p := range platform.Core {

			pkg, err := NewPackage(localManifest, p, ref)
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

func getDigest(l *ui.UI, state *pstate.State, pkg *Package, ref Reference) (string, error) {
	task := l.Task(ref.String())

	digest, err := pstate.CacheAndDigest(state, task, pkg)
	if err != nil {
		return "", errors.WithStack(err)
	}
	task.Done()
	return digest, nil

}
