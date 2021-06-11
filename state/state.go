package state

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/pkg/errors"
	"github.com/qdm12/reprint"

	"github.com/cashapp/hermit/archive"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/internal/dao"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
	"github.com/cashapp/hermit/vfs"
)

// DefaultSources if no others are defined.
var DefaultSources = []string{"https://github.com/cashapp/hermit-packages.git"}

type precompiledAutoMirror struct {
	re     *regexp.Regexp
	groups map[string]int
	mirror string
}

// AutoMirror defines a dynamically generated mirror URL mapping.
type AutoMirror struct {
	// Origin URL regex to generate a mirror from. Named patterns will be substituted into mirror.
	Origin string
	// Mirror URL to add.
	Mirror string
}

// Config for Hermit's global state.
type Config struct {
	// List of sources to sync packages from.
	Sources []string
	// Auto-generated mirrors.
	AutoMirrors []AutoMirror
	// Builtin sources.
	Builtin *sources.BuiltInSource
}

// State is the global hermit state shared between all local environments
type State struct {
	root        string // Path to the state directory
	cacheDir    string // Path to the root of the Hermit cache.
	pkgDir      string // Path to unpacked packages.
	sourcesDir  string // Path to extracted sources.
	config      Config
	autoMirrors []precompiledAutoMirror
	cache       *cache.Cache
	dao         *dao.DAO
	lock        *util.FileLock
}

// Open the global Hermit state.
func Open(stateDir string, config Config, client *http.Client, fastFailClient *http.Client) (*State, error) {
	if config.Builtin == nil {
		return nil, errors.Errorf("state.Config.Builtin not provided")
	}

	pkgDir := filepath.Join(stateDir, "pkg")
	cacheDir := filepath.Join(stateDir, "cache")
	sourcesDir := filepath.Join(stateDir, "sources")
	cache, err := cache.Open(cacheDir, client, fastFailClient)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	dao := dao.Open(stateDir)
	if config.Sources == nil {
		config.Sources = DefaultSources
	}

	// Validate and compile the auto-mirrors.
	autoMirrors := []precompiledAutoMirror{}
	for _, mirror := range config.AutoMirrors {
		re, err := regexp.Compile(mirror.Origin)
		if err != nil {
			return nil, errors.Errorf("auto-mirror key %q is not a valid regular expression", mirror.Origin)
		}
		pam := precompiledAutoMirror{
			re:     re,
			mirror: mirror.Mirror,
			groups: map[string]int{},
		}
		for id, name := range re.SubexpNames() {
			pam.groups[name] = id
		}
		os.Expand(mirror.Mirror, func(name string) string {
			_, ok := pam.groups[name]
			if !ok {
				err = errors.Errorf("unknown capture group %q in auto-mirror", name)
			}
			return ""
		})
		if err != nil {
			return nil, errors.WithStack(err)
		}
		autoMirrors = append(autoMirrors, pam)
	}

	s := &State{
		dao:         dao,
		autoMirrors: autoMirrors,
		root:        stateDir,
		cacheDir:    cacheDir,
		sourcesDir:  sourcesDir,
		config:      config,
		pkgDir:      pkgDir,
		cache:       cache,
		lock:        util.NewLock(filepath.Join(stateDir, ".lock"), 1*time.Second),
	}
	return s, nil
}

// Resolve package reference without an active environment.
func (s *State) Resolve(l *ui.UI, mathcer manifest.Selector) (*manifest.Package, error) {
	resolver, err := s.resolver(l)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return resolver.Resolve(l, mathcer)
}

// Search for packages without an active environment.
func (s *State) Search(l *ui.UI, glob string) (manifest.Packages, error) {
	resolver, err := s.resolver(l)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	pkgs, err := resolver.Search(l, glob)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return pkgs, nil
}

// DumpDB of State to w.
func (s *State) DumpDB(w io.Writer) error {
	return s.dao.Dump(w)
}

// Config returns the configuration stored in the global state.
func (s *State) Config() Config {
	return s.config
}

// SourcesDir returns the global directory for manifests
func (s *State) SourcesDir() string {
	return s.sourcesDir
}

// Root returns the root directory for the hermit state
func (s *State) Root() string {
	return s.root
}

// PkgDir returns path to the directory for extracted packages
func (s *State) PkgDir() string {
	return s.pkgDir
}

func (s *State) resolver(l *ui.UI) (*manifest.Resolver, error) {
	ss, err := sources.ForURIs(l, s.SourcesDir(), "", s.config.Sources)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	ss.Prepend(s.config.Builtin)
	return manifest.New(ss, manifest.Config{
		Env:   "",
		State: s.Root(),
		OS:    runtime.GOOS,
		Arch:  runtime.GOARCH,
	})
}

func (s *State) acquireLock(log ui.Logger) (*util.FileLock, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	err := s.lock.Acquire(ctx, log)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	return s.lock, nil
}

// ReadPackageState updates the package fields from the global database
func (s *State) ReadPackageState(pkg *manifest.Package) {
	if _, err := os.Stat(pkg.Root); err == nil {
		pkg.State = manifest.PackageStateInstalled
	} else if s.cache.IsCached(pkg.SHA256, pkg.Source) {
		pkg.State = manifest.PackageStateDownloaded
	}
	// We are ignoring the error as we might be updating a non exiting package
	dbInfo, _ := s.dao.GetPackage(pkg.String())
	if dbInfo == nil {
		dbInfo = &dao.Package{}
	}

	pkg.LastUsed = dbInfo.UsedAt
	pkg.ETag = dbInfo.Etag
	pkg.UpdatedAt = dbInfo.UpdateCheckedAt
}

// WritePackageState updates the fields and usage time stamp of the given package
func (s *State) WritePackageState(p *manifest.Package, binDir string) error {
	var updatedAt = time.Time{}
	if p.UpdateInterval > 0 {
		updatedAt = p.UpdatedAt
	}
	pkg := &dao.Package{
		UsedAt:          time.Now(),
		Etag:            p.ETag,
		UpdateCheckedAt: updatedAt,
	}
	return s.dao.UpdatePackageWithUsage(binDir, p.Reference.String(), pkg)
}

func (s *State) removePackage(b *ui.Task, pkg *manifest.Package) error {
	task := b.SubTask("remove")
	lock, err := s.acquireLock(b)
	if err != nil {
		return errors.WithStack(err)
	}
	defer lock.Release(b)

	task.Debugf("chmod -R +w %s", pkg.Dest)
	_ = filepath.Walk(pkg.Dest, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}
		return os.Chmod(path, info.Mode()|0200)
	})
	task.Debugf("rm -rf %s", pkg.Dest)

	return errors.WithStack(os.RemoveAll(pkg.Dest))
}

// CacheAndUnpack downloads a package and extracts it if it is not present.
// If the package has already been extracted, this is a no-op
func (s *State) CacheAndUnpack(b *ui.Task, p *manifest.Package) error {
	var (
		path string
		etag string
		err  error
	)

	// Check if the package is up-to-date, and if so, return before acquiring the lock
	if (s.isExtracted(p)) || p.Source == "/" {
		return nil
	}

	lock, err := s.acquireLock(b)
	if err != nil {
		return errors.WithStack(err)
	}
	defer lock.Release(b)

	if (s.isExtracted(p)) || p.Source == "/" {
		return nil
	}

	if !s.isCached(p) {
		mirrors := append(p.Mirrors, s.generateMirrors(p.Source)...)
		path, etag, err = s.cache.Download(b, p.SHA256, p.Source, mirrors...)
		p.ETag = etag
		if err != nil {
			return errors.WithStack(err)
		}
	} else {
		path = s.cache.Path(p.SHA256, p.Source)
	}

	err = archive.Extract(b, path, p)
	if err != nil {
		return errors.WithStack(err)
	}
	// Copy manifest referred files
	for _, file := range p.Files {
		err = vfs.CopyFile(file.FS, file.FromPath, file.ToPAth)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	if _, err = p.Trigger(b, manifest.EventUnpack); err != nil {
		_ = os.RemoveAll(p.Dest)
		return errors.WithStack(err)
	}
	return nil
}

func (s *State) isCached(p *manifest.Package) bool {
	return s.cache.IsCached(p.SHA256, p.Source)
}

func (s *State) isExtracted(p *manifest.Package) bool {
	_, err := os.Stat(p.Root)
	return err == nil
}

// RecordUninstall updates the package usage records of a package being uninstalled
func (s *State) RecordUninstall(pkg *manifest.Package, binDir string) error {
	err := s.dao.PackageRemovedAt(pkg.Reference.String(), binDir)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// CleanPackages removes all the extracted packages
func (s *State) CleanPackages(b ui.Logger) error {
	// TODO: Uninstall packages from their configured root so that eg. external packages can be uninstalled.
	lock, err := s.acquireLock(b)
	if err != nil {
		return err
	}
	defer lock.Release(b)

	b.Debugf("rm -rf %q", s.pkgDir)
	return os.RemoveAll(s.pkgDir)
}

// CleanCache clears the download cache
func (s *State) CleanCache(b ui.Logger) error {
	lock, err := s.acquireLock(b)
	if err != nil {
		return err
	}
	defer lock.Release(b)

	b.Debugf("rm -rf %q", s.cacheDir)
	return os.RemoveAll(s.cacheDir)
}

// UpgradeChannel checks if the given binary has changed in its channel, and if so, downloads it.
//
// If the channel is upgraded this will return a clone of the updated manifest.
func (s *State) UpgradeChannel(b *ui.Task, pkg *manifest.Package) (*manifest.Package, error) {
	if !pkg.Reference.IsChannel() {
		panic("UpgradeChannel can only be used with channel packages")
	}

	name := pkg.Reference.String()
	mirrors := append(pkg.Mirrors, s.generateMirrors(pkg.Source)...)
	etag, err := s.cache.ETag(b, pkg.Source, mirrors...)
	var updated *manifest.Package

	if err != nil {
		b.Warnf("Could not check updates for %s. Skipping update. Error: %s", name, err)
	} else if etag == "" {
		b.Warnf("No ETag found for %s. Skipping update.", name)
	} else if etag != pkg.ETag {
		b.Infof("Fetching a new version for %s", name)
		if err := s.evictPackage(b, pkg); err != nil {
			return nil, errors.WithStack(err)
		}
		if err := s.CacheAndUnpack(b, pkg); err != nil {
			return nil, errors.WithStack(err)
		}
		etag = pkg.ETag
		updated = reprint.This(pkg).(*manifest.Package)
		updated.UpdatedAt = time.Now()
	}

	dpkg := &dao.Package{
		UsedAt:          time.Now(),
		Etag:            etag,
		UpdateCheckedAt: time.Now(),
	}
	err = s.dao.UpdatePackage(name, dpkg)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return updated, nil
}

// GC clears packages that have not been used for the given duration and are not referred to in any environment
func (s *State) GC(p *ui.UI, age time.Duration, pkgResolver func(b *ui.UI, selector manifest.Selector, syncOnMissing bool) (*manifest.Package, error)) error {
	lock, err := s.acquireLock(p)
	if err != nil {
		return err
	}
	defer lock.Release(p)

	err = s.CleanCache(p)
	if err != nil {
		return errors.WithStack(err)
	}

	unused, err := s.dao.GetUnusedSince(time.Now().UTC().Add(-age))
	if err != nil {
		return errors.WithStack(err)
	}
	for _, name := range unused {
		task := p.Task(name)
		binDirs, err := s.dao.GetKnownUsages(name)
		if err != nil {
			return errors.WithStack(err)
		}
		inUse := false
		for _, binDir := range binDirs {
			exists, err := doesPackageExistAt(name, binDir)
			if err != nil {
				return errors.WithStack(err)
			}
			if exists {
				inUse = true
				continue
			}
			err = s.dao.PackageRemovedAt(name, binDir)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		if inUse {
			continue
		}

		task.Infof("Clearing %s", name)
		err = s.dao.DeletePackage(name)
		if err != nil {
			return errors.WithStack(err)
		}
		pkg, err := pkgResolver(p, manifest.ExactSelector(manifest.ParseReference(name)), false)
		// This can occur if a package was at some point installed and tracked
		// by the DB but now no longer exists in the manifests.
		if errors.Is(err, manifest.ErrUnknownPackage) {
			// TODO: Save paths to on-disk resources to the DB and remove them here as well
			continue
		}
		if err != nil {
			return errors.WithStack(err)
		}
		err = s.removePackage(task, pkg)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (s *State) evictPackage(b *ui.Task, pkg *manifest.Package) error {
	lock, err := s.acquireLock(b)
	if err != nil {
		return errors.WithStack(err)
	}
	defer lock.Release(b)

	if err := s.cache.Evict(b, pkg.SHA256, pkg.Source); err != nil {
		return errors.WithStack(err)
	}
	if err := s.removePackage(b, pkg); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Return the generated mirrors that match URL.
func (s *State) generateMirrors(url string) (mirrors []string) {
	for _, pam := range s.autoMirrors {
		matches := pam.re.FindStringSubmatch(url)
		if matches == nil {
			continue
		}
		mirror := os.Expand(pam.mirror, func(key string) string {
			return matches[pam.groups[key]]
		})
		mirrors = append(mirrors, mirror)
	}
	return
}

func doesPackageExistAt(name string, binDir string) (bool, error) {
	file := filepath.Join(binDir, "."+name+".pkg")
	_, err := os.Stat(file)
	if err != nil && !os.IsNotExist(err) {
		return false, errors.WithStack(err)
	} else if err == nil {
		return true, nil
	}
	return false, nil
}
