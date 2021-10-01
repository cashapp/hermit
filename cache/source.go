package cache

import (
	"context"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
	"github.com/pkg/errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type packageSource interface {
	OpenLocal(cache *Cache, checksum string) (*os.File, error)
	Download(b *ui.Task, cache *Cache, checksum string) (path string, etag string, err error)
	ETag(b *ui.Task, cache *Cache) (etag string, err error)
}

func getSource(uri string) (packageSource, error) {
	if strings.HasSuffix(uri, ".git") {
		return &gitSource{URL: uri}, nil
	}

	u, err := url.Parse(uri)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	switch u.Scheme {
	case "", "file":
		return &fileSource{Path: u.Path}, nil

	case "http", "https":
		return &httpSource{uri}, errors.WithStack(err)
	default:
		return nil, errors.Errorf("unsupported URI %s", uri)
	}
}

type fileSource struct {
	Path string
}

func (s *fileSource) OpenLocal(_ *Cache, _ string) (*os.File, error) {
	f, err := os.Open(s.Path)
	return f, errors.WithStack(err)
}

func (s *fileSource) Download(_ *ui.Task, _ *Cache, _ string) (path string, etag string, err error) {
	// TODO: Checksum it again?
	// Local file, just open it.
	return s.Path, "", nil
}

func (s *fileSource) ETag(_ *ui.Task, _ *Cache) (etag string, err error) {
	return "", nil
}

type httpSource struct {
	URL string
}

func (s *httpSource) OpenLocal(c *Cache, checksum string) (*os.File, error) {
	f, err := os.Open(c.Path(checksum, s.URL))
	return f, errors.WithStack(err)
}

func (s *httpSource) Download(b *ui.Task, cache *Cache, checksum string) (path string, etag string, err error) {
	cachePath := cache.Path(checksum, s.URL)
	return cache.downloadHTTP(b, checksum, s.URL, cachePath)
}

func (s *httpSource) ETag(_ *ui.Task, c *Cache) (etag string, err error) {
	uri := s.URL
	req, err := http.NewRequestWithContext(context.Background(), http.MethodHead, uri, nil)
	if err != nil {
		return "", errors.Wrap(err, uri)
	}
	resp, err := c.fastFailHTTPClient.Do(req)
	if err != nil {
		return "", errors.Wrap(err, uri)
	}
	defer resp.Body.Close()
	// Normal HTTP error, log and try the next mirror.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.Errorf("%s failed: %d", uri, resp.StatusCode)
	}
	result := resp.Header.Get("ETag")
	return result, nil
}

type gitSource struct {
	URL string
}

func (s *gitSource) OpenLocal(c *Cache, checksum string) (*os.File, error) {
	f, err := os.Open(c.Path(checksum, s.URL))
	return f, errors.WithStack(err)
}

func (s *gitSource) Download(b *ui.Task, cache *Cache, checksum string) (string, string, error) {
	base := BasePath(checksum, s.URL)
	err := util.RunInDir(b, cache.root, "git", "clone", "--depth=1", s.URL, base)
	if err != nil {
		return "", "", errors.Wrap(err, s.URL)
	}
	etag, err := s.ETag(b, cache)
	if err != nil {
		return "", "", errors.Wrap(err, s.URL)
	}

	return filepath.Join(cache.root, base), etag, nil
}

func (s *gitSource) ETag(b *ui.Task, c *Cache) (etag string, err error) {
	bts, err := util.Capture(b, "git", "ls-remote", s.URL, "HEAD")
	if err != nil {
		return "", errors.Wrap(err, s.URL)
	}
	str := string(bts)
	parts := strings.Split(str, "\t")
	if len(parts) != 2 {
		return "", errors.Errorf("invalid HEAD: %s", str)
	}

	return parts[0], nil
}
