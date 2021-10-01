package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"

	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

// Cache manages the Hermit cache.
type Cache struct {
	root               string
	httpClient         *http.Client
	gh                 *github.Client
	fastFailHTTPClient *http.Client
}

// BasePath returns the subfolder in the cache path for the given file
func BasePath(checksum, uri string) string {
	hash := util.Hash(uri, checksum)
	return filepath.Join(hash[:2], hash+"-"+filepath.Base(uri))
}

// Open or create a Cache at the given directory, using the given http client.
//
// "fastFailClient" is a HTTP client configured to fail quickly if a remote
// server is unavailable, for use in optional checks.
func Open(root string, ghClient *github.Client, client *http.Client, fastFailClient *http.Client) (*Cache, error) {
	err := os.MkdirAll(root, os.ModePerm)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &Cache{
		root:               root,
		httpClient:         client,
		gh:                 ghClient,
		fastFailHTTPClient: fastFailClient,
	}, nil
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
	source, err := GetSource(uri)
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
	for _, uri := range uris {
		defer ui.LogElapsed(b, "Download %s", uri)()
		source, err := GetSource(uri)
		if err != nil {
			return "", "", errors.WithStack(err)
		}
		path, etag, err = source.Download(b, c, checksum)
		if err == nil {
			return path, etag, nil
		}
		b.Debugf("%s: %s", uri, err)
	}
	if err == nil {
		return "", "", errors.Errorf("failed to download from any of %s", strings.Join(uris, ", "))
	}
	return "", "", errors.WithStack(err)
}

// ETag fetches the etag from given URI if available.
// Otherwise an empty string is returned
func (c *Cache) ETag(b *ui.Task, uri string, mirrors ...string) (etag string, err error) {
	for _, uri := range append([]string{uri}, mirrors...) {
		source, err := GetSource(uri)
		if err != nil {
			return "", errors.WithStack(err)
		}
		result, err := source.ETag(b, c)
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
	b.SubTask("remove").Debugf("rm -f %s", c.Path(checksum, uri))
	err := os.Remove(c.Path(checksum, uri))
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

func (c *Cache) downloadHTTP(b *ui.Task, checksum string, uri string, cachePath string) (string, string, error) {
	task := b.SubTask("download")
	downloadPath := cachePath + ".hermit.tmp.download"
	_ = os.MkdirAll(filepath.Dir(cachePath), os.ModePerm)

	// For HTTP files we download and cache them, then return the cached file.
	task.Debugf("Downloading %s", uri)

	w, response, err := Download(c.httpClient, nil, uri, downloadPath)
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	// Check for potential private github release
	if response.StatusCode == 404 {
		if ghi, ok := getGithubReleaseInfo(uri); ok {
			_ = response.Body.Close()
			_ = w.Close()
			w, response, err = downloadGHPrivate(c.gh, ghi, downloadPath)
			if err != nil {
				return "", "", errors.WithStack(err)
			}
			defer response.Body.Close()
		}
	}
	defer response.Body.Close()
	defer w.Close() // nolint: gosec
	if response.StatusCode < 200 || response.StatusCode > 299 {
		_ = os.Remove(w.Name())
		return "", "", errors.Errorf("failed to download %s: %s", uri, response.Status)
	}
	etag := response.Header.Get("ETag")

	info, err := w.Stat()
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	resumed := info.Size()
	task.Size(int(response.ContentLength + resumed))
	task.Add(int(resumed))
	defer task.Done()

	h := sha256.New()
	r := io.TeeReader(response.Body, h)
	r = io.TeeReader(r, task.ProgressWriter())
	_, err = io.Copy(w, r)
	if err != nil {
		_ = w.Close()
		return "", "", errors.WithStack(err)
	}
	err = w.Close()
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	// TODO: We'll need to checksum the existing content when resuming.
	actualChecksum := hex.EncodeToString(h.Sum(nil))
	if checksum != "" && checksum != actualChecksum {
		return "", "", errors.Errorf("%s: checksum %s should have been %s", uri, actualChecksum, checksum)
	}

	err = response.Body.Close()
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	// We finally have the checksummed file, move it into place.
	err = os.Rename(w.Name(), cachePath)
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	return cachePath, etag, nil
}

type writer interface {
	io.Writer
	io.WriterAt
}

// An io.WriterAt that updates a UI task's progress with the bytes written.
type progressWriterAt struct {
	w    writer
	task *ui.Task
}

func (p2 *progressWriterAt) Write(p []byte) (n int, err error) {
	n, err = p2.w.Write(p)
	p2.task.Add(n)
	return n, errors.WithStack(err)
}

func (p2 *progressWriterAt) WriteAt(p []byte, off int64) (n int, err error) {
	n, err = p2.w.WriteAt(p, off)
	p2.task.Add(n)
	return n, errors.WithStack(err)
}

// matches: https://github.com/{OWNER}/{REPO}/releases/download/{TAG}/{ASSET}
var githubRe = regexp.MustCompile(`^https\://github.com/([^/]+)/([^/]+)/releases/download/([^/]+)/([^/]+)$`)

type githubReleaseInfo struct {
	owner, repo, tag, asset string
}

func getGithubReleaseInfo(uri string) (*githubReleaseInfo, bool) {
	g := &githubReleaseInfo{}
	m := githubRe.FindStringSubmatch(uri)
	if len(m) != 5 {
		return nil, false
	}
	var err error
	if g.owner, err = url.PathUnescape(m[1]); err != nil {
		return nil, false
	}
	if g.repo, err = url.PathUnescape(m[2]); err != nil {
		return nil, false
	}
	if g.tag, err = url.PathUnescape(m[3]); err != nil {
		return nil, false
	}
	if g.asset, err = url.PathUnescape(m[4]); err != nil {
		return nil, false
	}
	return g, true
}

func downloadGHPrivate(client *github.Client, ghi *githubReleaseInfo, file string) (w *os.File, response *http.Response, err error) {
	r, err := client.Releases(fmt.Sprintf("%s/%s", ghi.owner, ghi.repo))
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	asset, err := getAssetURL(r, ghi.tag, ghi.asset)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	hc, req, err := client.PrepareDownload(asset)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	return Download(hc, req.Header, req.RequestURI, file)
}

func getAssetURL(releases []github.Release, tag, assetName string) (github.Asset, error) {
	for _, r := range releases {
		if r.TagName != tag {
			continue
		}
		for _, a := range r.Assets {
			if a.Name == assetName {
				return a, nil
			}
		}
	}
	return github.Asset{}, errors.Errorf("cannot find asset %s %s", tag, assetName)
}
