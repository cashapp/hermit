package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

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
	This command will build a list of installed packages and there checksums(SHA256) values.
	
	That file can be manually reviewed by the project owner and checked in to the repo.

	On subsequent uses of the hermit environment, the installed dependencies will have a reference checksum

	value for hermit to validate against.
	`
}

func (e *lockDigestsCmd) Run(l *ui.UI, env *hermit.Env, cache *cache.Cache, state *state.State) error {

	installed, err := env.ListInstalledReferences()
	if err != nil {
		return errors.WithStack(err)
	}
	if len(installed) == 0 {
		fmt.Printf("There are no packages installed. Nothing to do here.")
		return nil
	}
	for _, ref := range installed {
		task := l.Task(ref.String())
		pkg, err := env.Resolve(l, manifest.ExactSelector(ref), false)
		if err != nil {
			return errors.WithStack(err)
		}
		// This will have updated the checksum in the pkg object.
		err = state.CacheAndUnpack(task, pkg)
		filelocal := state.GetLocalFile(pkg.SHA256, pkg.Source)
		data, err := os.ReadFile(filelocal)
		if err != nil {
			return errors.WithStack(err)
		}
		h := sha256.New()
		h.Write(data)
		checksum := h.Sum(nil)
		//infer the manifest file
		res, err := env.GetResolver(l, manifest.ExactSelector(ref), true)
		if err != nil {
			return errors.WithStack(err)
		}
		localmanifest, err := res.ResolveManifest(l, manifest.ExactSelector(ref))
		if err != nil {
			return errors.WithStack(err)
		}
		if localmanifest.Manifest.SHA256Sums == nil {
			localmanifest.Manifest.SHA256Sums = make(map[string]string)
		}
		localmanifest.Manifest.SHA256Sums[pkg.Source] = hex.EncodeToString(checksum)
		// TODO: prune unecessary versions if needed ?
		value, _ := hcl.Marshal(localmanifest.Manifest)

		//Now just write down fresh manifest files in a subdirectory in the `bin`.
		os.MkdirAll(filepath.Join(env.BinDir(), "lockedDigests"), os.ModePerm)
		os.WriteFile(filepath.Join(env.BinDir(), "lockedDigests", pkg.Reference.Name+".hcl"), value, os.ModePerm)

		pkg.LogWarnings(l)
		task.Done()
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}
