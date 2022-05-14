package cache

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cashapp/hermit/errors"
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
	hash := util.Hash(uri, checksum)
	return filepath.Join(hash[:2], hash+"-"+filepath.Base(uri))
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
	err := os.MkdirAll(stateDir, os.ModePerm)
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
	path := c.Path("", uri)
	return path, os.MkdirAll(path, os.ModePerm)
}

// Create a new, empty, cache entry.
func (c *Cache) Create(checksum, uri string) (*os.File, error) {
	path := c.Path(checksum, uri)
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return os.Create(path)
}

// OpenLocal opens a local cached copy of "uri", or errors.
func (c *Cache) OpenLocal(checksum, uri string) (*os.File, error) {
	source, err := c.GetSource(c.httpClient, uri)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return source.OpenLocal(c, checksum)
}

// Open a local or remote artifact, transparently caching it. Subsequent accesses will use the cached copy.
//
// If checksum is present it must be the SHA256 hash of the downloaded artifact.
func (c *Cache) Open(b *ui.Task, checksum, uri string, mirrors ...string) (*os.File, error) {
	cachePath := c.Path(checksum, uri)
	_, err := os.Stat(cachePath)
	if err == nil {
		b.Tracef("returning cached path %s for %s", cachePath, uri)
		// TODO: Checksum it again?
		return os.Open(cachePath)
	} else if !os.IsNotExist(err) {
		return nil, errors.WithStack(err)
	}

	// No local cached copy, download it.
	path, _, err := c.Download(b, checksum, uri, mirrors...)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

// Download a local or remote artifact, transparently caching it.
//
// If checksum is present it must be the SHA256 hash of the downloaded artifact.
func (c *Cache) Download(b *ui.Task, checksum, uri string, mirrors ...string) (path string, etag string, err error) {
	uris := append([]string{uri}, mirrors...)
	var lastError error
	attempts := 3
	for attempt := 1; attempt <= attempts; attempt++ {
		for _, uri := range uris {
			defer ui.LogElapsed(b, "Download %s", uri)()
			source, err := c.GetSource(c.httpClient, uri)
			if err != nil {
				return "", "", errors.WithStack(err)
			}
			path, etag, err = source.Download(b, c, checksum)
			if err == nil {
				return path, etag, nil
			}
			lastError = err
			b.Debugf("%s: %s", uri, err)
		}
		if lastError == nil {
			return "", "", errors.Errorf("failed to download from any of %s", strings.Join(uris, ", "))
		}
		msg := fmt.Sprintf("Failed to download any of %s on attempt %d/%d: %s", strings.Join(uris, ", "), attempt, attempts, lastError)
		if attempt < attempts {
			msg = "Retrying. " + msg
		}
		b.Warnf("%s", msg)
		if attempt < attempts {
			time.Sleep(time.Second)
		}
	}
	return "", "", errors.Wrap(lastError, uris[len(uris)-1])
}

// ETag fetches the etag from given URI if available.
// Otherwise an empty string is returned
func (c *Cache) ETag(b *ui.Task, uri string, mirrors ...string) (etag string, err error) {
	for _, uri := range append([]string{uri}, mirrors...) {
		source, err := c.GetSource(c.fastFailHTTPClient, uri)
		if err != nil {
			return "", errors.WithStack(err)
		}
		result, err := source.ETag(b)
		if err != nil {
			b.Debugf("%s failed: %s", uri, err)
			continue
		}
		return result, nil
	}
	return "", nil
}

// IsCached returns true if the URI is cached.
func (c *Cache) IsCached(checksum, uri string) bool {
	_, err := os.Stat(c.Path(checksum, uri))
	return err == nil
}

// Evict a file from the cache.
func (c *Cache) Evict(b *ui.Task, checksum, uri string) error {
	b.SubTask("remove").Debugf("rm -rf %s", c.Path(checksum, uri))
	err := os.RemoveAll(c.Path(checksum, uri))
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
	base := BasePath(checksum, uri)
	return filepath.Join(c.root, base)
}

func (c *Cache) defaultDownloadStrategy(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, &bytes.Reader{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "default HTTP client failed")
	}
	return resp, nil
}
