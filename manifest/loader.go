package manifest

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/hcl"
	"github.com/gobwas/glob"
	"golang.org/x/sync/errgroup"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
)

// AnnotatedManifest includes extra metadata not included in the manifest itself.
type AnnotatedManifest struct {
	FS        fs.FS
	Path      string // Fully qualified path to manifest, including the FS.
	Name      string
	Errors    []error
	*Manifest // May be nil if errors were encountered.
}

func (f *AnnotatedManifest) String() string { return f.Path }

// ManifestErrors are collection of errors for named manifests
type ManifestErrors map[string][]error //nolint:revive

// LogErrors to the given logger
func (merrors ManifestErrors) LogErrors(l ui.Logger) {
	for fullPath, errors := range merrors {
		for _, e := range errors {
			l.Warnf("invalid manifest %s: %s", fullPath, e)
		}
	}
}

// Loader of manifests.
type Loader struct {
	lock    sync.Mutex
	sources *sources.Sources
	files   map[string]*AnnotatedManifest
}

// NewLoader constructs a new Loader.
func NewLoader(sources *sources.Sources) *Loader {
	return &Loader{
		sources: sources,
		files:   map[string]*AnnotatedManifest{},
	}
}

func (l *Loader) get(name string) (*AnnotatedManifest, error) {
	l.lock.Lock()
	defer l.lock.Unlock()
	// If we have already loaded it, just return it.
	file, ok := l.files[name]
	if !ok {
		path := name + ".hcl"
		// unavailable records the first sources.ErrSourceUnavailable seen
		// while searching, but we keep searching the remaining bundles: one
		// transiently-unavailable source must never mask a package provided
		// by another, healthy source.
		var unavailable error
		for _, bundle := range l.sources.Bundles() {
			f, err := load(bundle, name, path)
			if err != nil {
				if unavailable == nil {
					unavailable = err
				}
				continue
			}
			if f == nil {
				continue
			}
			file = f
			if unavailable == nil {
				// Only cache the result once every bundle consulted ahead of
				// it, in preference order, was actually reachable. If a
				// higher-preference bundle was unavailable, this answer may
				// be shadowed by that bundle's own manifest once it
				// recovers -- caching it here would let a transient outage
				// permanently invert source precedence for the rest of this
				// process's lifetime. Leaving it uncached means the next
				// lookup re-tries the unavailable bundle from scratch, so it
				// self-heals as soon as that bundle recovers.
				l.files[name] = file
			}
			break
		}
		// Only report unavailability if the manifest was found nowhere else.
		// Callers (Load) use this to distinguish "retry, this was
		// inconclusive" from a genuine ErrUnknownPackage.
		if file == nil && unavailable != nil {
			return nil, unavailable
		}
	}
	if file == nil {
		return nil, errors.Wrap(ErrUnknownPackage, l.unknownPackageDetail(name))
	}
	if len(file.Errors) > 0 {
		return nil, errors.WithStack(file.Errors[0])
	}
	return file, nil
}

// unknownPackageDetail enumerates the sources consulted when a package could
// not be found in any of them. Without this, a permanently misconfigured or
// inaccessible source (a bad "sources = [...]" entry, the wrong
// HERMIT_STATE_DIR, or a permissions problem) masquerades as "unknown
// package" for every package name, with no indication of why.
func (l *Loader) unknownPackageDetail(name string) string {
	return fmt.Sprintf("%s (searched %s)", name, strings.Join(l.sources.Sources(), ", "))
}

// Load a manifest for the given package.
// Syncs the sources if the manifest is not initially found.
// Will return a wrapped ErrUnknownPackage if the package could not be found.
//
// If any errors occur during the load, the first error will be returned.
func (l *Loader) Load(u *ui.UI, name string) (*AnnotatedManifest, error) {
	mnf, err := l.get(name)
	if err != nil {
		// Actively sync before falling back to sleeping through the bounded
		// backoff below: a source that has never been cloned needs a real
		// sync to ever become available, and sleeping first would add up to
		// ~620ms of pure latency to every such cold start for no benefit --
		// nothing changes the source's state on its own. This also covers
		// the genuinely-unknown-package case, in case the source's cache is
		// simply stale.
		if syncErr := l.sources.Sync(u, true); syncErr != nil {
			return nil, errors.WithStack(syncErr)
		}
		mnf, err = l.get(name)
	}
	// sourceUnavailableRetryBackoff bounds how long we'll additionally wait
	// for a transiently-unavailable source (see sources.ErrSourceUnavailable,
	// eg. another Hermit process mid-sync) to become available by itself --
	// useful when our own Sync call above was a no-op (Sources.Sync skips
	// sources once any one of them reports success) but a sibling process's
	// concurrent sync of this specific source finishes in the meantime.
	// Total worst case is ~620ms, deliberately short so a genuinely unknown
	// package is never delayed by it.
	sourceUnavailableRetryBackoff := []time.Duration{20 * time.Millisecond, 100 * time.Millisecond, 500 * time.Millisecond}
	for _, backoff := range sourceUnavailableRetryBackoff {
		if !errors.Is(err, sources.ErrSourceUnavailable) {
			break
		}
		time.Sleep(backoff)
		mnf, err = l.get(name)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return mnf, nil
}

// All loads all package manifests and returns them.
//
// Non-critical errors will be made available in each AnnotatedManifest and
// also via Errors().
func (l *Loader) All() ([]*AnnotatedManifest, error) {
	return l.Glob("*")
}

// Glob loads all package manifests based on the given glob and returns them.
//
// Non-critical errors will be made available in each AnnotatedManifest and
// also via Errors().
func (l *Loader) Glob(glob string) ([]*AnnotatedManifest, error) {
	l.lock.Lock()
	defer l.lock.Unlock()
	var (
		manifests []*AnnotatedManifest
		seen      = map[string]bool{}
	)

	type result struct {
		mft  *AnnotatedManifest
		name string
	}
	mftC := make(chan result)
	allDone := make(chan struct{})
	mu := sync.Mutex{}

	go func() {
		for t := range mftC {
			mu.Lock()
			l.files[t.name] = t.mft
			if t.mft.Manifest != nil {
				manifests = append(manifests, t.mft)
			}
			mu.Unlock()
		}
		close(allDone)
	}()

	wg := errgroup.Group{}
	// Throttle concurrency to avoid being too resource-greedy.
	wg.SetLimit(max(3, runtime.NumCPU()/4))

	pattern := glob + ".hcl"

	for _, bundle := range l.sources.Bundles() {
		files, err := fs.Glob(bundle, pattern)
		if err != nil {
			return nil, errors.Wrapf(err, "%s", bundle)
		}
		for _, file := range files {
			name := strings.TrimSuffix(file, ".hcl")
			if seen[name] {
				continue
			}
			seen[name] = true

			mu.Lock()
			if manifest, ok := l.files[name]; ok {
				manifests = append(manifests, manifest)
				mu.Unlock()
				continue
			}
			mu.Unlock()

			wg.Go(func() error {
				manifest, err := load(bundle, name, file)
				if err != nil {
					// A transiently-unavailable source isn't fatal here:
					// unlike Load, Glob/All are best-effort enumerations
					// across every bundle, so just skip what this one
					// bundle couldn't provide right now.
					return nil //nolint:nilerr
				}
				if manifest != nil {
					mftC <- result{manifest, name}
				}
				return nil
			})
		}
	}

	_ = wg.Wait()
	close(mftC)
	<-allDone

	return manifests, nil
}

// Errors returns all errors encountered _so far_ by the Loader.
func (l *Loader) Errors() ManifestErrors {
	l.lock.Lock()
	defer l.lock.Unlock()
	errors := ManifestErrors{}
	for _, file := range l.files {
		if len(file.Errors) > 0 {
			errors[file.String()] = append(errors[file.String()], file.Errors...)
		}
	}
	return errors
}

// Load manifest from bundle.
//
// Returns (nil, nil) if the manifest genuinely does not exist in this
// bundle. Returns a non-nil error wrapping sources.ErrSourceUnavailable if
// this bundle's backing source could not be read at all (eg. because
// another process is mid-sync) -- callers should treat that as
// "inconclusive", not "not found here".
func load(bundle fs.FS, name, filename string) (*AnnotatedManifest, error) {
	annotated := &AnnotatedManifest{
		FS:   bundle,
		Name: name,
		Path: fmt.Sprintf("%s/%s", bundle, filename),
	}
	data, err := fs.ReadFile(bundle, filename)
	switch {
	case errors.Is(err, sources.ErrSourceUnavailable):
		return nil, errors.WithStack(err)
	case errors.Is(err, os.ErrNotExist):
		return nil, nil
	case err != nil:
		annotated.Errors = append(annotated.Errors, errors.WithStack(err))
		return annotated, nil
	}
	manifest := &Manifest{}
	err = hcl.Unmarshal(data, manifest)
	if err != nil {
		annotated.Errors = append(annotated.Errors, errors.WithStack(err))
		return annotated, nil
	}
	annotated.Manifest = manifest
	annotated.Errors = append(annotated.Errors, annotated.validate()...)
	synthesise(annotated)
	return annotated, nil
}

// LoadManifestFile Utility function to just load a manifest file.
func LoadManifestFile(dir fs.FS, path string) (*AnnotatedManifest, error) {
	annotated := &AnnotatedManifest{
		FS:   dir,
		Name: strings.TrimSuffix(filepath.Base(path), ".hcl"),
		Path: fmt.Sprintf("%s/%s", dir, path),
	}
	data, err := fs.ReadFile(dir, path)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	manifest := &Manifest{}
	err = hcl.Unmarshal(data, manifest)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	annotated.Manifest = manifest

	return annotated, nil
}

// Synthesise a "stable" channel and a channel for each major version.
func synthesise(manifest *AnnotatedManifest) {
	highest, version := manifest.HighestMatch(glob.MustCompile("*"))
	if highest != nil && manifest.ChannelByName("latest") == nil {
		vstr := version.Major().String() + ".*"
		manifest.Channels = append(manifest.Channels, ChannelBlock{
			Name:    "latest",
			Update:  time.Hour * 24,
			Version: vstr,
		})
	}

	// Synthesise major and minor version channels.

	// Order the stable versions
	var versions Versions
	for _, block := range manifest.Versions {
		for _, vstr := range block.Version {
			blockVersion := ParseVersion(vstr)
			if blockVersion.Prerelease() != "" {
				continue
			}
			versions = append(versions, blockVersion)
		}
	}
	if len(versions) == 0 {
		return
	}
	sort.Sort(versions)

	channels := make([]string, 0, len(versions))
	seen := make(map[string]bool, len(versions))
	for _, version := range versions {
		major := version.Major().Clean().String()
		if !seen[major] && major != version.Clean().String() {
			seen[major] = true
			channels = append(channels, major)
		}
		majorMinor := version.MajorMinor().Clean().String()
		if !seen[majorMinor] && majorMinor != version.Clean().String() {
			seen[majorMinor] = true
			channels = append(channels, majorMinor)
		}
	}

	for _, version := range channels {
		manifest.Channels = append(manifest.Channels, ChannelBlock{
			Name:    version,
			Update:  time.Hour * 24,
			Version: version + ".*",
		})
	}
}
