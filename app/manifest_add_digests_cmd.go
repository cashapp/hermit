package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cashapp/hermit/manifest/manifestutils"

	"github.com/alecthomas/hcl"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
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
		base := filepath.Base(f)
		task := l.Task(packageName)
		var localmanifest *manifest.AnnotatedManifest
		localmanifest, err = manifest.LoadManifestFile(os.DirFS(dir), packageName, base)
		if err != nil {
			return errors.WithStack(err)
		}

		err = manifestutils.PopulateDigests(l, state, localmanifest)
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
			return errors.Wrapf(err, "Could not write the manifest file %s", f)
		}

		task.Done()
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}
	return nil
}
