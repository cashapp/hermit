package util

import (
	"os"
	"strings"
)

// CWD for computing RelPathCWD().
//
// We do this once at init time, but we also make it public so we can update it
// with flags if necessary.
var CWD = func() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return dir + "/"
}()

// RelPathCWD returns "path" relative to the current working directory.
func RelPathCWD(path string) string {
	return strings.TrimPrefix(path, CWD)
}

// RelPathsCWD returns "paths" relative to the current working directory.
func RelPathsCWD(paths []string) []string {
	out := make([]string, len(paths))
	for i, path := range paths {
		out[i] = strings.TrimPrefix(path, CWD)
	}
	return out
}

// Ext returns the full extension of a path.
func Ext(path string) string {
	dot := strings.IndexRune(path, '.')
	if dot == -1 {
		return ""
	}
	return path[dot:]
}
