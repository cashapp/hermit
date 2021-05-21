package util

import (
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// RealPath converts a path into its absolute, symlink-expanded form.
func RealPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if expanded, err := filepath.EvalSymlinks(path); err == nil {
		path = expanded
	}
	return path
}

// GlobOne globs exactly one file.
func GlobOne(glob string) (string, error) {
	matches, err := filepath.Glob(glob)
	if err != nil {
		return "", errors.WithStack(err)
	}
	if len(matches) != 1 {
		return "", errors.Errorf("expected exactly one file but found %s", strings.Join(matches, ", "))
	}
	return matches[0], nil
}
