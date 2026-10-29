package hermit

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/cashapp/hermit/envars"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/ui"
)

// linkFarm is an immutable directory of symlinks to package binaries, put on PATH ahead of bin/.
// Only "hermit clean" deletes farms, because bash caches command paths and fails if they disappear.
type linkFarm struct {
	dir   string
	links map[string]string // Binary name to absolute binary path.
}

// linkFarmsDir returns the directory holding all of this environment's farms.
// The path hash keeps environments with the same base name apart.
func (e *Env) linkFarmsDir() string {
	sum := sha256.Sum256([]byte(e.envDir))
	alias := filepath.Base(e.envDir) + "-" + hex.EncodeToString(sum[:])[:12]
	return filepath.Join(e.state.EnvironmentsDir(), alias)
}

// linkFarm computes the farm for the given installed packages without creating it.
// Only binaries bin/ links to a package are included, so the farm exposes nothing the stubs don't.
func (e *Env) linkFarm(installed []*manifest.Package) (*linkFarm, error) {
	linked, err := e.linkedBinaryNames()
	if err != nil {
		return nil, err
	}
	activated := e.activatedEnvars(installed)
	links := map[string]string{}
	for _, pkg := range installed {
		names := linked[pkg.String()]
		if len(names) == 0 || requiresStub(pkg) || !requirementsUnpacked(pkg, installed, map[string]bool{}) {
			continue
		}
		// Another package may override one of this package's variables, so only its stub
		// gives it its own values.
		if e.stubChangesEnvars(activated, installed, nil, pkg) {
			continue
		}
		// A package whose binaries cannot be resolved is left to its stub,
		// which reports the problem when run.
		binaries, _ := pkg.ResolveBinaries()
		for _, binary := range binaries {
			name := filepath.Base(binary)
			if _, ok := links[name]; !ok && slices.Contains(names, name) {
				links[name] = binary
			}
		}
	}

	names := make([]string, 0, len(links))
	for name := range links {
		names = append(names, name)
	}
	sort.Strings(names)
	hasher := sha256.New()
	for _, name := range names {
		hasher.Write([]byte(name + "\x00" + links[name] + "\x00"))
	}
	return &linkFarm{
		dir:   filepath.Join(e.linkFarmsDir(), hex.EncodeToString(hasher.Sum(nil))[:16]),
		links: links,
	}, nil
}

// createLinkFarm creates the farm for the given installed packages before it is put on PATH.
// It first checks linked channel packages for updates, because their stubs no longer run to do it.
func (e *Env) createLinkFarm(l *ui.UI, installed []*manifest.Package) (*linkFarm, error) {
	for _, pkg := range installed {
		if requiresStub(pkg) {
			continue
		}
		if err := e.EnsureChannelIsUpToDate(l, pkg); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	farm, err := e.linkFarm(installed)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if err := farm.create(); err != nil {
		return nil, errors.WithStack(err)
	}
	return farm, nil
}

// requiresStub reports whether a package needs the work its bin/ stubs do at execution time.
func requiresStub(pkg *manifest.Package) bool {
	switch {
	case pkg.State != manifest.PackageStateInstalled:
		// The stub downloads and unpacks the package on first use.
		return true
	case pkg.Unsupported(), len(pkg.Triggers[manifest.EventExec]) > 0, len(pkg.RuntimeDeps) > 0:
		return true
	}
	// The stub adds a package's own Hermit environment to PATH.
	for _, script := range []string{"hermit", "activate-hermit"} {
		if _, err := os.Lstat(filepath.Join(pkg.Root, "bin", script)); err == nil {
			return true
		}
	}
	return false
}

// activatedEnvars returns the variables activation gives a shell, apart from the link farm.
func (e *Env) activatedEnvars(installed []*manifest.Package) envars.Envars {
	system := envars.Parse(os.Environ())
	return system.Apply(e.Root(), e.allEnvarOpsForPackages("", nil, nil, installed...)).Combined()
}

// stubChangesEnvars reports whether pkg's stub runs it with different variables than activation sets.
func (e *Env) stubChangesEnvars(activated envars.Envars, installed, runtimeDeps []*manifest.Package, pkg *manifest.Package, extra ...envars.Op) bool {
	system := envars.Parse(os.Environ())
	ops := append(e.allEnvarOpsForPackages("", runtimeDeps, pkg, installed...), extra...)
	return !sameEnvars(activated, system.Apply(e.Root(), ops).Combined())
}

// requirementsUnpacked reports whether every installed package pkg requires, directly or not, is unpacked.
// Its stub unpacks them first, and a linked binary may reach them through variables such as JAVA_HOME.
func requirementsUnpacked(pkg *manifest.Package, installed []*manifest.Package, seen map[string]bool) bool {
	for _, req := range pkg.Requires {
		selector, err := manifest.ParseGlobSelector(req)
		for _, dep := range installed {
			ref := dep.Reference.String()
			if seen[ref] || !(slices.Contains(dep.Provides, req) || (err == nil && selector.Matches(dep.Reference))) {
				continue
			}
			seen[ref] = true
			if dep.State != manifest.PackageStateInstalled || !requirementsUnpacked(dep, installed, seen) {
				return false
			}
		}
	}
	return true
}

// sameEnvars reports whether a binary would see the same variables in both environments.
// List order is ignored because stubs only reorder PATH entries, never drop them.
func sameEnvars(a, b envars.Envars) bool {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	for k := range keys {
		if strings.HasPrefix(k, "_HERMIT_OLD_") {
			continue
		}
		av, aok := a[k]
		bv, bok := b[k]
		if aok != bok {
			return false
		}
		if av == bv {
			continue
		}
		al, bl := strings.Split(av, ":"), strings.Split(bv, ":")
		sort.Strings(al)
		sort.Strings(bl)
		if !slices.Equal(al, bl) {
			return false
		}
	}
	return true
}

// create writes the farm if it does not exist, renaming a temporary directory into place
// so concurrent activations never see a partial farm.
func (f *linkFarm) create() error {
	if _, err := os.Lstat(f.dir); err == nil {
		return nil
	}
	parent := filepath.Dir(f.dir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return errors.WithStack(err)
	}
	tmp, err := os.MkdirTemp(parent, ".tmp-")
	if err != nil {
		return errors.WithStack(err)
	}
	for name, binary := range f.links {
		if err := os.Symlink(binary, filepath.Join(tmp, name)); err != nil {
			_ = os.RemoveAll(tmp)
			return errors.WithStack(err)
		}
	}
	if err := os.Rename(tmp, f.dir); err != nil {
		_ = os.RemoveAll(tmp)
		// Another activation created the same farm first.
		if _, statErr := os.Lstat(f.dir); statErr == nil {
			return nil
		}
		return errors.WithStack(err)
	}
	return nil
}
