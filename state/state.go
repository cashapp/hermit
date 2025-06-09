package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/cashapp/hermit/archive"
	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/internal/dao"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/platform"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
	"github.com/cashapp/hermit/util/flock"
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
	Builtin     *sources.BuiltInSource
	LockTimeout time.Duration
}

// State is the global hermit state shared between all local environments
type State struct {
	root        string // Path to the state directory
	cacheDir    string // Path to the root of the Hermit cache.
	pkgDir      string // Path to unpacked packages.
	sourcesDir  string // Path to extracted sources.
	binaryDir   string // Path to directory with symlinks to package binaries
	config      Config
	autoMirrors []precompiledAutoMirror
	cache       *cache.Cache
	dao         *dao.DAO
	lock        string
	lockTimeout time.Duration
}

// Open the global Hermit state.
//
// See cache.Open for details on downloadStrategies.
func Open(stateDir string, config Config, cache *cache.Cache) (*State, error) {
	if config.Builtin == nil {
		return nil, errors.Errorf("state.Config.Builtin not provided")
	}

	pkgDir := filepath.Join(stateDir, "pkg")
	cacheDir := filepath.Join(stateDir, "cache")
	sourcesDir := filepath.Join(stateDir, "sources")
	binaryDir := filepath.Join(stateDir, "binaries")
	dao, err := dao.Open(stateDir)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if config.Sources == nil {
		config.Sources = DefaultSources
	}

	autoMirrors, err := validateAndCompileAutoMirrors(config)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	s := &State{
		dao:         dao,
		autoMirrors: autoMirrors,
		root:        stateDir,
		cacheDir:    cacheDir,
		sourcesDir:  sourcesDir,
		binaryDir:   binaryDir,
		config:      config,
		pkgDir:      pkgDir,
		cache:       cache,
		lock:        filepath.Join(stateDir, ".lock"),
		lockTimeout: config.LockTimeout,
	}
	return s, nil
}

func validateAndCompileAutoMirrors(config Config) ([]precompiledAutoMirror, error) {
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
	return autoMirrors, nil
}

// Resolve package reference without an active environment.
func (s *State) Resolve(l *ui.UI, matcher manifest.Selector) (*manifest.Package, error) {
	resolver, err := s.resolver(l)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return resolver.Resolve(l, matcher)
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

// BinaryDir returns path to the directory for bin symlinks
func (s *State) BinaryDir() string {
	return s.binaryDir
}

func (s *State) resolver(l *ui.UI) (*manifest.Resolver, error) {
	ss, err := s.Sources(l)
	if err != nil {
		return nil, err
	}
	return manifest.New(ss, manifest.Config{
		State: s.Root(),
		Platform: platform.Platform{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
	})
}

// Sources associated with the State.
func (s *State) Sources(l *ui.UI) (*sources.Sources, error) {
	ss, err := sources.ForURIs(l, s.SourcesDir(), "", s.config.Sources)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	ss.Prepend(s.config.Builtin)
	return ss, nil
}

func (s *State) acquireLock(log ui.Logger, format string, args ...any) (release func() error, err error) {
	log.Tracef("timeout for acquiring the lock is %s", s.lockTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), s.lockTimeout)
	release, err = flock.Acquire(ctx, s.lock, fmt.Sprintf(format, args...))
	cancel()
	if err != nil {
		return nil, errors.Wrap(err, "failed to acquire lock")
	}
	return
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

	pkg.ETag = dbInfo.Etag
	pkg.UpdatedAt = dbInfo.UpdateCheckedAt
}

// WritePackageState updates the fields and usage time stamp of the given package
func (s *State) WritePackageState(p *manifest.Package) error {
	var updatedAt = time.Time{}
	if p.UpdateInterval > 0 {
		updatedAt = p.UpdatedAt
	}
	pkg := &dao.Package{
		Etag:            p.ETag,
		UpdateCheckedAt: updatedAt,
	}
	return s.dao.UpdatePackage(p.Reference.String(), pkg)
}

func (s *State) removeRecursive(b *ui.Task, dest string) error {
	_, err := os.Stat(dest)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.WithStack(err)
	}

	task := b.SubTask("remove")
	release, err := s.acquireLock(b, "recursively removing %s", dest)
	if err != nil {
		return errors.WithStack(err)
	}
	defer release() //nolint:errcheck

	task.Debugf("chmod -R +w %s", dest)
	_ = filepath.Walk(dest, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}
		err = os.Chmod(path, info.Mode()|0200)

		if errors.Is(err, os.ErrNotExist) {
			task.Debugf("file did not exist during removal %q", path)
			return nil
		}
		return errors.WithStack(err)
	})
	task.Debugf("rm -rf %s", dest)
	return errors.WithStack(os.RemoveAll(dest))
}

// CacheAndUnpack downloads a package and extracts it if it is not present.
//
// If the package has already been extracted, this is a no-op
func (s *State) CacheAndUnpack(b *ui.Task, p *manifest.Package) error {
	// Double-checked locking. We check without the lock first, and then check
	// again after acquiring the lock.

	if (s.isExtracted(p) && s.areBinariesLinked(p)) || p.Source == "/" {
		return nil
	}

	release, err := s.acquireLock(b, "downloading and extracting %s", p)
	if err != nil {
		return errors.WithStack(err)
	}
	defer release() //nolint:errcheck

	if !s.isExtracted(p) {
		if err := s.extract(b, p); err != nil {
			return errors.WithStack(err)
		}
	}

	if !s.areBinariesLinked(p) {
		if err := s.linkBinaries(p); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// CacheAndDigest Utility for Caching all platform artefacts.
//
// This method will only cache the values and get a digest.
func (s *State) CacheAndDigest(b *ui.Task, p *manifest.Package) (string, error) {
	var actualDigest string
	var err error
	if !s.isCached(p) {
		mirrors := make([]string, len(p.Mirrors))
		copy(mirrors, p.Mirrors)
		mirrors = append(mirrors, s.generateMirrors(p.Source)...)
		_, _, actualDigest, err = s.cache.Download(b, p.SHA256, p.Source, mirrors...)
		if err != nil {
			return "", errors.WithStack(err)
		}
	} else if p.SHA256 != "" {
		// If the manifest has SHA256 value then the package installation must have
		// checked that. So just use it.
		actualDigest = p.SHA256
	} else {
		// if the artifact is cached then just calculate the digest.
		path := s.cache.Path(p.SHA256, p.Source)
		actualDigest, err = util.Sha256LocalFile(path)
		if err != nil {
			return "", errors.WithStack(err)
		}
	}
	return actualDigest, nil
}

func (s *State) linkBinaries(p *manifest.Package) error {
	dir := filepath.Join(s.binaryDir, p.Reference.String())
	// clean up the binaryDir before
	if err := os.RemoveAll(dir); err != nil {
		return errors.WithStack(err)
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.WithStack(err)
	}

	bins, err := p.ResolveBinaries()
	if err != nil {
		return errors.WithStack(err)
	}

	for _, bin := range bins {
		to := filepath.Join(dir, filepath.Base(bin))

		if dest, err := os.Readlink(to); err == nil && dest == bin {
			continue
		}

		if err := os.Symlink(bin, to); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (s *State) extract(b *ui.Task, p *manifest.Package) error {
	var (
		path string
		etag string
		err  error
	)

	if !s.isCached(p) {
		mirrors := make([]string, len(p.Mirrors))
		copy(mirrors, p.Mirrors)
		mirrors = append(mirrors, s.generateMirrors(p.Source)...)
		path, etag, _, err = s.cache.Download(b, p.SHA256, p.Source, mirrors...)
		p.ETag = etag

		if err != nil {
			return errors.WithStack(err)
		}
	} else {
		path = s.cache.Path(p.SHA256, p.Source)
	}

	finalise, err := archive.Extract(b, path, p)
	if err != nil {
		return errors.WithStack(err)
	}
	// Copy manifest referred files
	for _, file := range p.Files {
		err = vfs.CopyFile(file.FS, file.FromPath, file.ToPath)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	if _, err = p.Trigger(b, manifest.EventUnpack); err != nil {
		_ = os.RemoveAll(p.Dest)
		return errors.WithStack(err)
	}
	return errors.WithStack(finalise())
}

func (s *State) isCached(p *manifest.Package) bool {
	return s.cache.IsCached(p.SHA256, p.Source)
}

func (s *State) isExtracted(p *manifest.Package) bool {
	_, err := os.Stat(p.Root)
	return err == nil
}

func (s *State) areBinariesLinked(p *manifest.Package) bool {
	binaries, err := p.ResolveBinaries()
	if err != nil {
		return false
	}
	for _, bin := range binaries {
		linkPath := filepath.Join(s.binaryDir, p.Reference.String(), filepath.Base(bin))
		if _, err := os.Stat(linkPath); err != nil {
			return false
		}
		// also checks the link destination matches the binary path.
		ld, err := os.Readlink(linkPath)
		if err != nil {
			return false
		}

		if bin != ld {
			return false
		}
	}
	return true
}

// CleanPackages removes all extracted packages
func (s *State) CleanPackages(b *ui.UI) error {
	// TODO: Uninstall packages from their configured root so that eg. external packages can be uninstalled.
	release, err := s.acquireLock(b, "cleaning all extracted packages")
	if err != nil {
		return err
	}
	defer release() //nolint:errcheck

	bins, err := os.ReadDir(s.binaryDir)
	if err != nil && !os.IsNotExist(err) {
		return errors.WithStack(err)
	}
	for _, entry := range bins {
		path := filepath.Join(s.binaryDir, entry.Name())
		if err = s.removeRecursive(b.Task(entry.Name()), path); err != nil {
			return errors.WithStack(err)
		}
	}

	entries, err := os.ReadDir(s.pkgDir)
	if err != nil && !os.IsNotExist(err) {
		return errors.WithStack(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "hermit@") {
			continue
		}
		path := filepath.Join(s.pkgDir, entry.Name())
		if err = s.removeRecursive(b.Task(entry.Name()), path); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// CleanCache clears the download cache
func (s *State) CleanCache(b ui.Logger) error {
	release, err := s.acquireLock(b, "cleaning download cache")
	if err != nil {
		return err
	}
	defer release() //nolint:errcheck

	b.Debugf("rm -rf %q", s.cacheDir)
	return os.RemoveAll(s.cacheDir)
}

// UpgradeChannel checks if the given binary has changed in its channel, and if so, downloads it.
//
// If the channel is upgraded this will return a clone of the updated manifest.
func (s *State) UpgradeChannel(b *ui.Task, pkg *manifest.Package) error {
	if !pkg.Reference.IsChannel() {
		panic("UpgradeChannel can only be used with channel packages")
	}

	name := pkg.Reference.String()
	mirrors := make([]string, len(pkg.Mirrors))
	copy(mirrors, pkg.Mirrors)
	mirrors = append(mirrors, s.generateMirrors(pkg.Source)...)

	etag, err := s.cache.ETag(b, pkg.Source, mirrors...)
	if err != nil {
		b.Warnf("Could not check updates for %s. Skipping update. Error: %s", name, err)
	} else if etag == "" {
		b.Warnf("No ETag found for %s. Skipping update.", name)
	} else if etag != pkg.ETag {
		b.Infof("Fetching a new version for %s", name)
		if err := s.evictPackage(b, pkg); err != nil {
			return errors.WithStack(err)
		}
		if err := s.CacheAndUnpack(b, pkg); err != nil {
			return errors.WithStack(err)
		}
		etag = pkg.ETag
	} else {
		b.Infof("No update required")
	}

	if etag == "" {
		etag = pkg.ETag
	}

	pkg.UpdatedAt = time.Now()
	dpkg := &dao.Package{
		Etag:            etag,
		UpdateCheckedAt: time.Now(),
	}
	return errors.WithStack(s.dao.UpdatePackage(name, dpkg))
}

func (s *State) removePackage(task *ui.Task, pkg *manifest.Package) error {
	err := s.removeRecursive(task, filepath.Join(s.binaryDir, pkg.Reference.String()))
	if err != nil {
		return errors.WithStack(err)
	}
	// Evicting the current executing package may cause issues on certain filesystems (ex: NFS)
	// while the binary is in use. Instead of removing, we rename the package directory instead.
	if strings.HasPrefix(filepath.Base(pkg.Dest), "hermit@") {
		newPath := filepath.Join(s.pkgDir, fmt.Sprintf("%s.old", filepath.Base(pkg.Dest)))
		// If new path exists, remove it first
		if _, err := os.Stat(newPath); err == nil {
			s.removeRecursive(task, newPath)
		}
		task.Debugf("mv %s %s", pkg.Dest, newPath)
		return errors.WithStack(os.Rename(pkg.Dest, newPath))
	}
	err = s.removeRecursive(task, pkg.Dest)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (s *State) evictPackage(b *ui.Task, pkg *manifest.Package) error {
	release, err := s.acquireLock(b, "evicting package %s", pkg)
	if err != nil {
		return errors.WithStack(err)
	}
	defer release() //nolint:errcheck

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
