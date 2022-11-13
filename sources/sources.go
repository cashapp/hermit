package sources

import (
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cashapp/hermit/util"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
)

// SyncFrequency determines how frequently sources will be synced.
const SyncFrequency = time.Hour * 24

// Source is a single source for manifest files
type Source interface {
	// Sync synchronises these sources from the possibly remote origin
	Sync(p *ui.UI, force bool) error
	// URI returns a URI for the source
	URI() string
	// Bundle returns a fs.FS for the manifests from this source
	Bundle() fs.FS
}

// Sources knows how to sync manifests from various sources such as git repositories.
type Sources struct {
	sources        []Source
	dir            string
	isSynchronised bool // Keep track if the sources have been synchronised to avoid double synchronisation
}

// New returns a new set of sources
func New(stateDir string, sources []Source) *Sources {
	return &Sources{
		dir:     stateDir,
		sources: sources,
	}
}

// Prepend a new source
func (s *Sources) Prepend(source Source) {
	s.sources = append([]Source{source}, s.sources...)
}

// Add a new source
func (s *Sources) Add(source Source) {
	s.sources = append(s.sources, source)
}

// Sync synchronises manifests from remote repos.
// Will be synced at most every SyncFrequency unless "force" is true.
// A Sources set can only be synchronised once. Following calls will not have any effect.
func (s *Sources) Sync(p *ui.UI, force bool) error {
	if s.isSynchronised {
		return nil
	}
	s.isSynchronised = true
	for _, source := range s.sources {
		err := source.Sync(p, force)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// ForURIs returns Source instances for given uri strings
func ForURIs(b *ui.UI, dir, env string, uris []string) (*Sources, error) {
	sources := make([]Source, 0, len(uris))
	for _, uri := range uris {
		s, err := getSource(b, uri, dir, env)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if s != nil {
			sources = append(sources, s)
		}
	}
	return &Sources{
		dir:     dir,
		sources: sources,
	}, nil
}

func getSource(b *ui.UI, source, dir, env string) (Source, error) {
	task := b.Task(source)
	defer task.Done()

	if strings.HasSuffix(source, ".git") || strings.Contains(source, ".git#") {
		return NewGitSource(source, dir, &util.RealCommandRunner{}), nil
	}

	uri, err := url.Parse(source)
	if err != nil {
		return nil, errors.Wrap(err, "invalid source")
	}
	var (
		// Directory of source, if any, to check for existence.
		checkDir  string
		candidate fs.FS
	)
	switch uri.Scheme {
	case "env":
		if uri.Path == "" {
			task.Warnf("%s does not contain a path", uri)
			return nil, nil
		}
		checkDir = filepath.Join(env, uri.Path)
		candidate = os.DirFS(checkDir)

	case "file":
		if uri.Path == "" {
			task.Warnf("%s does not contain a path", uri)
			return nil, nil
		}
		checkDir = uri.Path
		candidate = os.DirFS(uri.Path)

	default:
		return nil, errors.Errorf("unsupported source %q", source)
	}
	if info, err := os.Stat(checkDir); err == nil {
		return NewLocalSource(source, candidate), nil
	} else if info != nil && !info.IsDir() {
		task.Warnf("source %q should be a directory but is not", source)
	} else {
		task.Warnf("source %q not found: %s", source, err)
	}
	return nil, nil
}

// Sources returns the source URIs
func (s *Sources) Sources() []string {
	combined := []string{}
	for _, s := range s.sources {
		combined = append(combined, s.URI())
	}
	return combined
}

// Bundles returns all the package manifests bundles
func (s *Sources) Bundles() []fs.FS {
	combined := []fs.FS{}
	for _, s := range s.sources {
		combined = append(combined, s.Bundle())
	}
	return combined
}

// This exists to provide useful debugging information back to the user.
type uriFS struct {
	uri string
	fs.FS
}

func (u *uriFS) Stat(name string) (fs.FileInfo, error)      { return fs.Stat(u.FS, name) }
func (u *uriFS) ReadDir(name string) ([]fs.DirEntry, error) { return fs.ReadDir(u.FS, name) }
func (u *uriFS) Glob(pattern string) ([]string, error)      { return fs.Glob(u.FS, pattern) }
func (u *uriFS) String() string                             { return u.uri }
