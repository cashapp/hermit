package app

import (
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
	//time.Sleep(time.Second * 7)
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
		//infer the manifest file
		res, err := env.GetResolver(l, manifest.ExactSelector(ref), true)
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
		for _, mc := range localmanifest.Manifest.Versions {
			//multiple platform definitions
			if len(mc.Layer.Platform) > 0 {
				for _, plat := range mc.Layer.Platform {
					// this is for cases where manifests are like:
					// version "11.0.11_9-zulu11.48.21" {
					//  platform darwin arm64 {
					if plat.Attrs[0] == "darwin" {
						arch := "amd64"
						if len(plat.Attrs) > 1 {
							arch = plat.Attrs[1]
						}
						ref = manifest.ParseReference(ref.Name + "-" + mc.Version[0])
						pkg, err := res.ResolveWithConfig(l, manifest.ExactSelector(ref), manifest.DarwinConfig(res.GetConfig(), arch))
						if err != nil {
							return errors.WithStack(err)
						}
						checksum, err := getSHASum(l, state, pkg, &ref)
						if err != nil {
							return errors.WithStack(err)
						}
						localmanifest.Manifest.SHA256Sums[pkg.Source] = checksum
						//plat.Layer.SHA256 = checksum

					}
					if plat.Attrs[0] == "linux" {
						ref = manifest.ParseReference(ref.Name + "-" + mc.Version[0])
						pkg, err := res.ResolveWithConfig(l, manifest.ExactSelector(ref), manifest.LinuxConfig(res.GetConfig()))
						if err != nil {
							return errors.WithStack(err)
						}
						checksum, err := getSHASum(l, state, pkg, &ref)
						if err != nil {
							return errors.WithStack(err)
						}
						//plat.Layer.SHA256 = checksum
						localmanifest.Manifest.SHA256Sums[pkg.Source] = checksum

					}
				}
			} else if len(mc.Layer.Darwin) > 0 {
				// an example is :https://github.com/cashapp/hermit-packages/blob/31f421d7396046f5fd296daa9239ecd1e2ba1d4b/openjdk.hcl#L33-L40
				ref = manifest.ParseReference(ref.Name + "-" + mc.Version[0])
				pkg, err := res.ResolveWithConfig(l, manifest.ExactSelector(ref), manifest.DarwinConfig(res.GetConfig(), "darwin"))
				if err != nil {
					return errors.WithStack(err)
				}
				checksum, err := getSHASum(l, state, pkg, &ref)
				if err != nil {
					return errors.WithStack(err)
				}
				localmanifest.Manifest.SHA256Sums[pkg.Source] = checksum

			} else if len(mc.Layer.Linux) > 0 {
				// an example is :https://github.com/cashapp/hermit-packages/blob/31f421d7396046f5fd296daa9239ecd1e2ba1d4b/openjdk.hcl#L33-L40
				ref = manifest.ParseReference(ref.Name + "-" + mc.Version[0])
				pkg, err := res.ResolveWithConfig(l, manifest.ExactSelector(ref), manifest.DarwinConfig(res.GetConfig(), "linux"))
				if err != nil {
					return errors.WithStack(err)
				}
				checksum, err := getSHASum(l, state, pkg, &ref)
				if err != nil {
					return errors.WithStack(err)
				}
				localmanifest.Manifest.SHA256Sums[pkg.Source] = checksum
			} else {
				// only version definitions
				ref = manifest.ParseReference(ref.Name + "-" + mc.Version[len(mc.Version)-1])
				pkg, err := res.Resolve(l, manifest.ExactSelector(ref))
				if err != nil {
					return errors.WithStack(err)
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
func getSHASum(l *ui.UI, state *state.State, pkg *manifest.Package, ref *manifest.Reference) (string, error) {
	task := l.Task(ref.String())

	checksum, err := state.CacheAndDontUnpack(task, pkg)
	if err != nil {
		return "", errors.WithStack(err)
	}
	task.Done()
	return checksum, nil

}
