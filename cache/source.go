package cache

import (
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/cashapp/hermit/util"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
)

// PackageSourceSelector selects a PackageSource for a URI.
//
// If not provided to the Cache, GetSource() will be used.
type PackageSourceSelector func(client *http.Client, uri string) (PackageSource, error)

// PackageSource for a specific version / system of a package
type PackageSource interface {
	OpenLocal(cache *Cache, checksum string) (*os.File, error)
	Download(b *ui.Task, cache *Cache, checksum string) (path string, etag string, actualChecksum string, err error)
	ETag(b *ui.Task) (etag string, err error)
	// Validate that a source is accessible.
	Validate() error
}

// GetSource for the given uri, or an error if the uri can not be parsed as a source
func GetSource(client *http.Client, uri string) (PackageSource, error) {
	if strings.HasSuffix(uri, ".git") || strings.Contains(uri, ".git#") {
		return &gitSource{URL: uri}, nil
	}

	u, err := url.Parse(uri)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	switch u.Scheme {
	case "", "file":
		return &fileSource{path: u.Path}, nil

	case "http", "https":
		return HTTPSource(client, uri), errors.WithStack(err)

	default:
		return nil, errors.Errorf("unsupported URI %s", uri)
	}
}

type fileSource struct {
	path string
}

func (s *fileSource) OpenLocal(_ *Cache, _ string) (*os.File, error) {
	f, err := os.Open(s.path)
	return f, errors.WithStack(err)
}

func (s *fileSource) Download(_ *ui.Task, _ *Cache, checksum string) (path string, etag string, actualChecksum string, err error) {
	info, err := os.Stat(s.path)
	if err != nil {
		return "", "", "", errors.WithStack(err)
	}
	// If the file is a directory then no checksum is required
	if info.IsDir() {
		return s.path, "", "", nil
	}
	var calculatedDigest string
	calculatedDigest, err = util.Sha256LocalFile(s.path)
	if err != nil {
		return "", "", "", errors.WithStack(err)
	}
	if checksum != "" && checksum != calculatedDigest {
		return "", "", "", errors.Errorf("%s: checksum %s should have been %s", s.path, calculatedDigest, checksum)
	}
	return s.path, "", calculatedDigest, nil
}

func (s *fileSource) ETag(b *ui.Task) (etag string, err error) {
	return "", nil
}

func (s *fileSource) Validate() error {
	_, err := os.Stat(s.path)
	return errors.Wrapf(err, "invalid file location")
}
