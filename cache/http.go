package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
)

type httpSource struct {
	client *http.Client
	url    string
}

// HTTPSource is a PackageSource for a HTTP URL.
func HTTPSource(client *http.Client, url string) PackageSource {
	return &httpSource{client, url}
}

func (s *httpSource) OpenLocal(c *Cache, checksum string) (*os.File, error) {
	f, err := os.Open(c.Path(checksum, s.url))
	return f, errors.WithStack(err)
}

func (s *httpSource) Download(b *ui.Task, cache *Cache, checksum string) (path string, etag string, actualChecksum string, err error) {
	cachePath := cache.Path(checksum, s.url)
	b.Tracef("cachePath %v checksum %v url %v \n", cachePath, checksum, s.url)
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, "GET", s.url, &bytes.Reader{})
	if err != nil {
		return "", "", "", errors.Wrap(err, "could not fetch")
	}
	response, err := s.client.Do(req)
	if err != nil {
		return "", "", "", errors.Wrap(err, "could not download to cache")
	}
	defer response.Body.Close()
	return downloadHTTP(b, response, checksum, s.url, cachePath)
}

func (s *httpSource) ETag(b *ui.Task) (etag string, err error) {
	uri := s.url
	req, err := http.NewRequestWithContext(context.Background(), http.MethodHead, uri, nil)
	if err != nil {
		return "", errors.Wrap(err, uri)
	}
	resp, err := s.client.Do(req)
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

func (s *httpSource) Validate() error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodHead, s.url, nil)
	if err != nil {
		return errors.Wrap(err, s.url)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return errors.WithStack(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return errors.Errorf("could not retrieve source archive from %s: %s", s.url, resp.Status)
	}

	return nil
}

func downloadHTTP(b *ui.Task, response *http.Response, checksum string, uri string, cachePath string) (path string, etag string, returnChecksum string, err error) {
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return "", "", "", errors.Errorf("download failed: %s (%d), source url: %s", response.Status, response.StatusCode, uri)
	}
	task := b.SubTask("download")
	cacheDir := filepath.Dir(cachePath)
	_ = os.MkdirAll(cacheDir, os.ModePerm) //nolint:gosec

	w, err := os.CreateTemp(cacheDir, filepath.Base(cachePath)+".*.hermit.tmp.download")
	if err != nil {
		return "", "", "", errors.Wrap(err, "couldn't create temporary for download")
	}
	defer w.Close() // nolint: gosec
	defer os.Remove(w.Name())

	// For HTTP files we download and cache them, then return the cached file.
	task.Debugf("Downloading %s", uri)

	etag = response.Header.Get("ETag")

	info, err := w.Stat()
	if err != nil {
		return "", "", "", errors.WithStack(err)
	}
	resumed := info.Size()
	task.Size(int(max(response.ContentLength + resumed, 0)))
	task.Add(int(resumed))
	defer task.Done()

	h := sha256.New()
	r := io.TeeReader(response.Body, h)
	r = io.TeeReader(r, task.ProgressWriter())
	_, err = io.Copy(w, r)
	if err != nil {
		_ = w.Close()
		return "", "", "", errors.WithStack(err)
	}
	err = w.Close()
	if err != nil {
		return "", "", "", errors.WithStack(err)
	}
	// TODO: We'll need to checksum the existing content when resuming.
	actualChecksum := hex.EncodeToString(h.Sum(nil))
	if checksum != "" && checksum != actualChecksum {
		return "", "", "", errors.Errorf("%s: checksum %s should have been %s", uri, actualChecksum, checksum)
	}

	err = response.Body.Close()
	if err != nil {
		return "", "", "", errors.WithStack(err)
	}

	// We finally have the checksummed file, move it into place.
	err = os.Rename(w.Name(), cachePath)
	if err != nil {
		return "", "", "", errors.WithStack(err)
	}
	return cachePath, etag, actualChecksum, nil
}
