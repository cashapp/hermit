package manifestutils

import (
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/platform"
	pstate "github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

// PopulateDigests Add missing digests to the manifest file.
func PopulateDigests(l *ui.UI, state *pstate.State, localManifest *manifest.AnnotatedManifest) error {
	_, _, err := PopulateDigestsForVersion(l, state, localManifest, nil)
	return err
}

// PopulateDigestsForVersion Add digest values for a specific version.
// We want to be able to use this in autoversion and manifest add-digests commands.
// For autoversion we might have an old and stale version lying around which will fail to download and break the command.
// To handle that case just calculate hashes for the new version.
func PopulateDigestsForVersion(l *ui.UI, state *pstate.State, localManifest *manifest.AnnotatedManifest, v *manifest.VersionBlock) ([]string, []string, error) {
	if len(localManifest.Manifest.Versions) != 0 && localManifest.Manifest.SHA256Sums == nil {
		localManifest.Manifest.SHA256Sums = make(map[string]string)
	}
	l.Infof("Working on %s package", localManifest.Name)

	var versions []manifest.VersionBlock
	if v == nil {
		versions = localManifest.Manifest.Versions
	} else {
		versions = append(versions, *v)
	}
	returnSource := make([]string, 0)
	returnDigest := make([]string, 0)
	for _, mc := range versions {
		ref := manifest.ParseReference(localManifest.Name + "-" + mc.Version[len(mc.Version)-1])
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
				return nil, nil, errors.WithStack(err)
			}

			localManifest.Manifest.SHA256Sums[pkg.Source] = digest
			if v != nil {
				returnSource = append(returnSource, pkg.Source)
				returnDigest = append(returnDigest, digest)
			}
		}
	}
	return returnSource, returnDigest, nil
}

func getDigest(l *ui.UI, state *pstate.State, pkg *manifest.Package, ref manifest.Reference) (string, error) {
	task := l.Task(ref.String())

	digest, err := state.CacheAndDigest(task, pkg)
	if err != nil {
		return "", errors.WithStack(err)
	}
	task.Done()
	return digest, nil

}
