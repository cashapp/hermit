package util

import (
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// URL is a builder with convenience methods for manipulating the path component of URLs.
//
// This exists because directly using filepath on a URL string makes it explode.
type URL struct {
	u url.URL
}

// ParseURL returns a fluent-style URL manipulator.
func ParseURL(uri string) (URL, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return URL{}, errors.WithStack(err)
	}
	return URL{*u}, nil
}

// Join a dir to the URL path.
func (u URL) Join(paths ...string) URL {
	u.u.Path = path.Join(append([]string{u.u.Path}, paths...)...)
	return u
}

// ReplaceExt replaces the path file extension with ext.
func (u URL) ReplaceExt(ext string) URL {
	u.u.Path = strings.TrimSuffix(u.u.Path, filepath.Ext(u.u.Path)) + ext
	return u
}

func (u URL) String() string {
	return u.u.String()
}

// Scheme of the URL
func (u URL) Scheme() string {
	return u.u.Scheme
}

// Path component of the URL.
func (u URL) Path() string {
	return u.u.Path
}
