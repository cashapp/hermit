package cache

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

// Cache manages the Hermit cache.
type Cache struct {
	// GetSource determines how to retrieve packages.
	GetSource PackageSourceSelector

	root               string
	httpClient         *http.Client
	fastFailHTTPClient *http.Client
}

// BasePath returns the subfolder in the cache path for the given file
func BasePath(checksum, uri string) string {
	return basePath(checksum, sources.NewSource(uri))
}

func basePath(checksum string, uri sources.Source) string {
	hash := util.Hash(uri.Get(), checksum)
	return filepath.Join(hash[:2], hash+"-"+filepath.Base(uri.Get()))
}

// Open or create a Cache at the given directory, using the given http client.
//
// "stateDir" is the root of the Hermit state directory.
//
// "selector" is used to select a PackageSource for retrieving packages
//
// "fastFailClient" is a HTTP client configured to fail quickly if a remote
// server is unavailable, for use in optional checks.
//
// "strategies" are used to download URLS, attempted in order.
// A default raw HTTP download strategy will always be the first strategy attempted.
func Open(stateDir string, selector PackageSourceSelector, client *http.Client, fastFailClient *http.Client) (*Cache, error) {
	stateDir = filepath.Join(stateDir, "cache")
	err := os.MkdirAll(stateDir, os.ModePerm) //nolint:gosec
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if selector == nil {
		selector = GetSource
	}
	c := &Cache{
		root:               stateDir,
		GetSource:          selector,
		httpClient:         client,
		fastFailHTTPClient: fastFailClient,
	}
	return c, nil
}

// Root directory of the cache.
func (c *Cache) Root() string {
	return c.root
}

// Mkdir makes a directory for the given URI.
func (c *Cache) Mkdir(uri string) (string, error) {
	path := c.path("", sources.NewSource(uri))
	return path, os.MkdirAll(path, os.ModePerm) //nolint:gosec
}

// Create a new, empty, cache entry.
func (c *Cache) Create(checksum, uri string) (*os.File, error) {
	path := c.path(checksum, sources.NewSource(uri))
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, os.ModePerm) //nolint:gosec
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return os.Create(path)
}

// OpenLocal opens a local cached copy of "uri", or errors.
func (c *Cache) OpenLocal(checksum, uri string) (*os.File, error) {
	source, err := c.GetSource(c.httpClient, sources.NewSource(uri))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return source.OpenLocal(c, checksum)
}

// Open a local or remote artifact, transparently caching it. Subsequent accesses will use the cached copy.
//
// If checksum is present it must be the SHA256 hash of the downloaded artifact.
func (c *Cache) Open(b *ui.Task, checksum, uri string, mirrors ...string) (*os.File, error) {
	source := sources.NewSource(uri)
	cachePath := c.path(checksum, source)
	_, err := os.Stat(cachePath)
	if err == nil {
		b.Tracef("returning cached path %s for %s", cachePath, source)
		// TODO: Checksum it again?
		return os.Open(cachePath)
	} else if !os.IsNotExist(err) {
		return nil, errors.WithStack(err)
	}

	// No local cached copy, download it.
	path, _, _, err := c.download(b, checksum, source, wrapSources(mirrors))
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

// Download a local or remote artifact, transparently caching it.
//
// If checksum is present it must be the SHA256 hash of the downloaded artifact.
func (c *Cache) Download(b *ui.Task, checksum, uri string, mirrors ...string) (path string, etag string, actualChecksum string, err error) {
	return c.download(b, checksum, sources.NewSource(uri), wrapSources(mirrors))
}

func (c *Cache) download(b *ui.Task, checksum string, uri sources.Source, mirrors []sources.Source) (path string, etag string, actualChecksum string, err error) {
	uris := append([]sources.Source{uri}, mirrors...)
	var lastError error
	attempts := 3
	for attempt := 1; attempt <= attempts; attempt++ {
		for _, sourceURI := range uris {
			defer ui.LogElapsed(b, "Download %s", sourceURI)()
			source, err := c.GetSource(c.httpClient, sourceURI)
			if err != nil {
				return "", "", "", errors.WithStack(err)
			}
			path, etag, actualChecksum, err = source.Download(b, c, checksum)
			if err == nil {
				return path, etag, actualChecksum, nil
			}
			lastError = err
			b.Debugf("%s: %s", sourceURI, err)
		}
		if lastError == nil {
			return "", "", "", errors.Errorf("failed to download from any of %s", joinSources(uris))
		}
		msg := fmt.Sprintf("Failed to download any of %s on attempt %d/%d: %s", joinSources(uris), attempt, attempts, lastError)
		if attempt < attempts {
			msg = "Retrying. " + msg
		}
		b.Warnf("%s", msg)
		if attempt < attempts {
			time.Sleep(time.Second)
		}
	}
	return "", "", "", &UnavailableError{
		URI: uris[len(uris)-1],
		Err: lastError,
	}
}

// ETag fetches the etag from given URI if available.
// Otherwise an empty string is returned
func (c *Cache) ETag(b *ui.Task, uri string, mirrors ...string) (etag string, err error) {
	return c.etag(b, sources.NewSource(uri), wrapSources(mirrors))
}

func (c *Cache) etag(b *ui.Task, uri sources.Source, mirrors []sources.Source) (etag string, err error) {
	for _, sourceURI := range append([]sources.Source{uri}, mirrors...) {
		source, err := c.GetSource(c.fastFailHTTPClient, sourceURI)
		if err != nil {
			return "", errors.WithStack(err)
		}
		result, err := source.ETag(b)
		if err != nil {
			b.Debugf("%s failed: %s", sourceURI, err)
			continue
		}
		return result, nil
	}
	return "", nil
}

// IsCached returns true if the URI is cached.
func (c *Cache) IsCached(checksum, uri string) bool {
	_, err := os.Stat(c.path(checksum, sources.NewSource(uri)))
	return err == nil
}

// Evict a file from the cache.
func (c *Cache) Evict(b *ui.Task, checksum, uri string) error {
	path := c.path(checksum, sources.NewSource(uri))
	b.SubTask("remove").Debugf("rm -rf %s", path)
	err := os.RemoveAll(path)
	if err != nil && !os.IsNotExist(err) {
		return errors.WithStack(err)
	}
	return nil
}

// Clean the cache.
func (c *Cache) Clean() error {
	return os.RemoveAll(c.root)
}

// Path to cached object.
func (c *Cache) Path(checksum, uri string) string {
	return c.path(checksum, sources.NewSource(uri))
}

func (c *Cache) path(checksum string, uri sources.Source) string {
	base := basePath(checksum, uri)
	return filepath.Join(c.root, base)
}

func wrapSources(raw []string) []sources.Source {
	wrapped := make([]sources.Source, len(raw))
	for i, uri := range raw {
		wrapped[i] = sources.NewSource(uri)
	}
	return wrapped
}

func joinSources(uris []sources.Source) string {
	redacted := make([]string, len(uris))
	for i, uri := range uris {
		redacted[i] = uri.String()
	}
	return strings.Join(redacted, ", ")
}

// UnavailableError returns 101 for the exit code.
type UnavailableError struct {
	URI sources.Source
	Err error
}

// Error returns the error string for the unavailable error
func (e *UnavailableError) Error() string {
	msg := e.URI.String() + " is unavailable"
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", msg, e.Err)
	}
	return msg
}

// ExitCode returns 101 as its exit code
func (e *UnavailableError) ExitCode() int {
	return 101
}
