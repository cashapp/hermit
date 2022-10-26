package manifest

import (
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/hcl"
	"github.com/gobwas/glob"

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
type ManifestErrors map[string][]error

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
		for _, bundle := range l.sources.Bundles() {
			file = load(bundle, name, path)
			if file == nil {
				continue
			}
			l.files[name] = file
			break
		}
	}
	if file == nil {
		return nil, errors.Wrap(ErrUnknownPackage, name)
	}
	if len(file.Errors) > 0 {
		return nil, errors.WithStack(file.Errors[0])
	}
	return file, nil
}

// Load a manifest for the given package.
// Syncs the sources if the manifest is not initially found.
// Will return a wrapped ErrUnknownPackage if the package could not be found.
//
// If any errors occur during the load, the first error will be returned.
func (l *Loader) Load(u *ui.UI, name string) (*AnnotatedManifest, error) {
	mnf, err := l.get(name)
	if err != nil {
		err := l.sources.Sync(u, true)
		if err != nil {
			return nil, errors.Wrap(err, err.Error())
		}
		// Try again.
		mnf, err = l.get(name)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return mnf, nil
}

// All loads all package manifests and returns them.
//
// Non-critical errors will be made available in each AnnotatedManifest and
// also via Errors().
func (l *Loader) All() ([]*AnnotatedManifest, error) {
	l.lock.Lock()
	defer l.lock.Unlock()
	var (
		manifests []*AnnotatedManifest
		seen      = map[string]bool{}
	)
	for _, bundle := range l.sources.Bundles() {
		files, err := fs.Glob(bundle, "*.hcl")
		if err != nil {
			return nil, errors.Wrapf(err, "%s", bundle)
		}
		for _, file := range files {
			name := strings.TrimSuffix(file, ".hcl")
			if seen[name] {
				continue
			}
			seen[name] = true
			if manifest, ok := l.files[name]; ok {
				manifests = append(manifests, manifest)
				continue
			}
			manifest := load(bundle, name, file)
			if manifest == nil {
				continue
			}
			l.files[name] = manifest
			if manifest.Manifest != nil {
				manifests = append(manifests, manifest)
			}
		}
	}
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
// Will return nil if it does not exist.
func load(bundle fs.FS, name, filename string) *AnnotatedManifest {
	annotated := &AnnotatedManifest{
		FS:   bundle,
		Name: name,
		Path: fmt.Sprintf("%s/%s", bundle, filename),
	}
	data, err := fs.ReadFile(bundle, filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		annotated.Errors = append(annotated.Errors, errors.WithStack(err))
		return annotated
	}
	manifest := &Manifest{}
	err = hcl.Unmarshal(data, manifest)
	if err != nil {
		annotated.Errors = append(annotated.Errors, errors.WithStack(err))
		return annotated
	}
	annotated.Manifest = manifest
	annotated.Errors = append(annotated.Errors, annotated.validate()...)
	synthesise(annotated)
	return annotated
}

// LoadManifestBytes Utility function to parse bytes.
func LoadManifestBytes(data []byte, annotated *AnnotatedManifest) (*AnnotatedManifest, error) {
	manifest := &Manifest{}
	err := hcl.Unmarshal(data, manifest)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	annotated.Manifest = manifest

	return annotated, nil
}

// LoadManifestFile Utility function to just load a manifest file.
func LoadManifestFile(dir fs.FS, name, filename string) (*AnnotatedManifest, error) {
	annotated := &AnnotatedManifest{
		FS:   dir,
		Name: name,
		Path: fmt.Sprintf("%s/%s", dir, filename),
	}
	data, err := fs.ReadFile(dir, filename)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return LoadManifestBytes(data, annotated)
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
