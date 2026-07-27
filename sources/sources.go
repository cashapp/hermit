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

// ErrSourceUnavailable indicates that a source's backing directory could not
// be found at all -- as opposed to the directory existing but simply not
// containing the requested manifest. Distinguishing the two matters because
// a git source's directory can be transiently absent while another Hermit
// process is mid-sync (see GitSource.Sync), which is not the same thing as
// "genuinely unknown package": callers should retry rather than treat it as
// authoritative.
var ErrSourceUnavailable = errors.New("source unavailable")

// Source is a single source for manifest files
type Source interface {
	// Sync synchronises these sources from the possibly remote origin.
	// Returns true if the source was actually updated.
	Sync(p *ui.UI, force bool) (bool, error)
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

func (s *Sources) LocalDirs() []string {
	var out []string
	for _, source := range s.sources {
		if local, ok := source.(*LocalSource); ok {
			dir := strings.TrimPrefix(local.fs.uri, "env:///")
			out = append(out, dir)
		}
	}
	return out
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
// Sources will only be synchronised once per invocation. Following calls will not have any effect.
func (s *Sources) Sync(p *ui.UI, force bool) error {
	if s.isSynchronised {
		return nil
	}
	synced := false
	for _, source := range s.sources {
		did, err := source.Sync(p, force)
		if err != nil {
			return errors.WithStack(err)
		}
		synced = synced || did
	}
	if synced {
		s.isSynchronised = true
	}
	return nil
}

// URLRewriter is a function that can transform a source URI
type URLRewriter func(uri string) (string, error)

// ForURIs returns Source instances for given uri strings
func ForURIs(b *ui.UI, dir, env string, uris []string, rewriters ...URLRewriter) (*Sources, error) {
	sources := make([]Source, 0, len(uris))
	for _, uri := range uris {
		// Apply each rewriter in sequence
		transformedURI := uri
		for _, rewrite := range rewriters {
			rewritten, err := rewrite(transformedURI)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			transformedURI = rewritten
		}

		s, err := getSource(b, transformedURI, dir, env)
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

	if strings.HasSuffix(source, ".git") {
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
	// dir, if set, is the backing directory on disk for this source. It is
	// used to distinguish "this manifest doesn't exist in this bundle" from
	// "this bundle's backing directory itself is currently missing" (eg.
	// because another process is mid-sync, or the source configuration or
	// permissions are wrong). Only set for sources actually backed by a
	// directory that can meaningfully vanish (GitSource): leaving it empty
	// for in-memory sources avoids misreporting them as unavailable, since
	// some (eg. vfs.InMemoryFS) return fs.ErrNotExist unconditionally.
	dir string
	fs.FS
}

func (u *uriFS) Stat(name string) (fs.FileInfo, error)      { return fs.Stat(u.FS, name) }
func (u *uriFS) ReadDir(name string) ([]fs.DirEntry, error) { return fs.ReadDir(u.FS, name) }
func (u *uriFS) Glob(pattern string) ([]string, error)      { return fs.Glob(u.FS, pattern) }
func (u *uriFS) String() string                             { return u.uri }

// Open wraps the underlying FS's Open, reporting ErrSourceUnavailable
// instead of the usual fs.ErrNotExist when the failure is because this
// source's entire backing directory is missing, rather than just the
// requested file within it.
//
// The os.Stat below is necessarily retrospective and best-effort: it checks
// whether the directory is missing *now*, not whether it was missing at the
// moment FS.Open failed above. A directory that vanishes and reappears
// between those two calls (eg. a fast concurrent resync) can still be
// misreported either way. That's fine for our purposes -- callers only use
// ErrSourceUnavailable as a signal to retry, never as an authoritative
// answer -- but it means this is a heuristic, not a guarantee.
func (u *uriFS) Open(name string) (fs.File, error) {
	f, err := u.FS.Open(name)
	if err != nil && u.dir != "" && errors.Is(err, fs.ErrNotExist) {
		if _, statErr := os.Stat(u.dir); os.IsNotExist(statErr) {
			return nil, errors.Wrap(ErrSourceUnavailable, u.uri)
		}
	}
	return f, err
}
