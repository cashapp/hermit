package vfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cashapp/hermit/errors"
)

// InMemoryFS creates a FS from a map of filename to content.
func InMemoryFS(files map[string]string) fs.GlobFS {
	return &inMemoryBundle{files: files}
}

type inMemoryBundle struct {
	files map[string]string
}

func (i inMemoryBundle) String() string {
	return "memory://"
}

func (i inMemoryBundle) Open(path string) (fs.File, error) {
	data, ok := i.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return inMemoryFile{path, len(data), strings.NewReader(data)}, nil
}

func (i inMemoryBundle) Glob(pattern string) ([]string, error) {
	var out []string
	for path := range i.files {
		if ok, err := filepath.Match(pattern, path); ok {
			out = append(out, path)
		} else if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return out, nil
}

type inMemoryFile struct {
	name   string
	length int
	*strings.Reader
}

func (i inMemoryFile) Close() error {
	i.Reader = nil
	return nil
}

func (i inMemoryFile) Stat() (fs.FileInfo, error) {
	return &fileStat{
		name:    i.name,
		size:    int64(i.length),
		mode:    os.ModePerm,
		modTime: time.Date(2018, 1, 1, 1, 1, 1, 1, time.UTC),
	}, nil
}

// A fileStat is the implementation of FileInfo returned by Stat and Lstat.
type fileStat struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

func (fs *fileStat) Name() string       { return fs.name }
func (fs *fileStat) IsDir() bool        { return false }
func (fs *fileStat) Size() int64        { return fs.size }
func (fs *fileStat) Mode() os.FileMode  { return fs.mode }
func (fs *fileStat) ModTime() time.Time { return fs.modTime }
func (fs *fileStat) Sys() interface{}   { return nil }
