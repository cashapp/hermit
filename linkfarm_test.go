package hermit_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/cashapp/hermit"
	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/hermittest"
	"github.com/cashapp/hermit/manifest"
)

func TestActivateLinksUnpackedBinariesIntoLinkFarm(t *testing.T) {
	// Each package archive contains a single binary named after its URL path.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tar := TestTarGz{map[string]string{strings.TrimPrefix(r.URL.Path, "/"): "#!/bin/sh\n"}}
		tar.Write(t, w)
	})
	fixture := hermittest.NewEnvTestFixture(t, handler)
	url := fixture.Server.URL
	fixture.WithManifests(map[string]string{
		"tool.hcl": `
			description = ""
			binaries = ["tool"]
			source = "` + url + `/tool"
			version "1.0.0" "1.0.1" {}
		`,
		"hooked.hcl": `
			description = ""
			binaries = ["hooked"]
			source = "` + url + `/hooked"
			on exec {
				message { text = "exec" }
			}
			version "1.0.0" {}
		`,
		"dep.hcl": `
			description = ""
			binaries = ["dep"]
			source = "` + url + `/dep"
			version "1.0.0" {}
		`,
		"withdep.hcl": `
			description = ""
			binaries = ["withdep"]
			source = "` + url + `/withdep"
			runtime-dependencies = ["dep-1.0.0"]
			version "1.0.0" {}
		`,
		"lazy.hcl": `
			description = ""
			binaries = ["lazy"]
			source = "` + url + `/lazy"
			version "1.0.0" {}
		`,
	})
	for _, ref := range []string{"tool-1.0.0", "hooked-1.0.0", "withdep-1.0.0"} {
		install(t, fixture, ref)
	}
	// Stubs without an unpacked package, as in a fresh clone.
	binDir := fixture.Env.BinDir()
	assert.NoError(t, os.Symlink("hermit", filepath.Join(binDir, ".lazy-1.0.0.pkg")))
	assert.NoError(t, os.Symlink(".lazy-1.0.0.pkg", filepath.Join(binDir, "lazy")))

	ops, err := fixture.Env.EnvOps(fixture.P)
	assert.NoError(t, err)
	farm := linkFarmDir(t, ops)
	assert.True(t, strings.HasPrefix(farm, fixture.State.EnvironmentsDir()+"/"), farm)
	_, err = os.Stat(farm)
	assert.True(t, os.IsNotExist(err), "EnvOps must not create the link farm")

	_, activateOps, err := fixture.Env.Activate(fixture.P)
	assert.NoError(t, err)
	assert.Equal(t, ops, activateOps)
	assert.Equal(t, map[string]string{
		"tool": filepath.Join(fixture.RootDir(), "tool-1.0.0", "tool"),
	}, readLinks(t, farm))
	vars, err := fixture.Env.Envars(fixture.P, false)
	assert.NoError(t, err)
	path := ""
	for _, v := range vars {
		if strings.HasPrefix(v, "PATH=") {
			path = strings.TrimPrefix(v, "PATH=")
		}
	}
	assert.True(t, strings.HasPrefix(path, farm+":"+binDir+":"), path)

	// A package change creates a new farm and leaves the old one untouched.
	install(t, fixture, "tool-1.0.1")
	_, activateOps, err = fixture.Env.Activate(fixture.P)
	assert.NoError(t, err)
	upgradedFarm := linkFarmDir(t, activateOps)
	assert.NotEqual(t, farm, upgradedFarm)
	assert.Equal(t, map[string]string{
		"tool": filepath.Join(fixture.RootDir(), "tool-1.0.1", "tool"),
	}, readLinks(t, upgradedFarm))
	assert.Equal(t, map[string]string{
		"tool": filepath.Join(fixture.RootDir(), "tool-1.0.0", "tool"),
	}, readLinks(t, farm))
}

func TestCleanRemovesLinkFarms(t *testing.T) {
	fixture := hermittest.NewEnvTestFixture(t, nil)
	_, ops, err := fixture.Env.Activate(fixture.P)
	assert.NoError(t, err)
	farm := linkFarmDir(t, ops)
	_, err = os.Stat(farm)
	assert.NoError(t, err)

	assert.NoError(t, fixture.Env.Clean(fixture.P, hermit.CleanBin))
	_, err = os.Stat(filepath.Dir(farm))
	assert.True(t, os.IsNotExist(err), "clean --bin must remove the environment's link farms")

	_, _, err = fixture.Env.Activate(fixture.P)
	assert.NoError(t, err)
	assert.NoError(t, fixture.Env.Clean(fixture.P, hermit.CleanPackages))
	entries, err := os.ReadDir(fixture.State.EnvironmentsDir())
	assert.NoError(t, err)
	assert.Equal(t, 0, len(entries))
}

func install(t *testing.T, fixture *hermittest.EnvTestFixture, ref string) {
	t.Helper()
	pkg, err := fixture.Env.Resolve(fixture.P, manifest.ExactSelector(manifest.ParseReference(ref)), false)
	assert.NoError(t, err)
	_, err = fixture.Env.Install(fixture.P, pkg)
	assert.NoError(t, err)
}

func linkFarmDir(t *testing.T, ops envars.Ops) string {
	t.Helper()
	for _, op := range ops {
		if force, ok := op.(*envars.Force); ok && force.Name == "HERMIT_LINK_FARM" {
			return force.Value
		}
	}
	t.Fatalf("HERMIT_LINK_FARM not set by %v", ops)
	return ""
}

func readLinks(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	assert.NoError(t, err)
	links := map[string]string{}
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join(dir, entry.Name()))
		assert.NoError(t, err)
		links[entry.Name()] = target
	}
	return links
}
