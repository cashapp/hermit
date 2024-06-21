package util

import (
	"os"
	"path/filepath"

	"github.com/cashapp/hermit/errors"
)

// ResolveSymlinks returns all symlinks in a chain, including the final file, as absolute paths.
func ResolveSymlinks(path string) (links []string, err error) {
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	links = append(links, path)
	var link string
	for range 20 {
		if info, err := os.Lstat(path); err != nil {
			return nil, errors.Wrap(err, path)
		} else if info.Mode()&os.ModeSymlink == 0 {
			break
		}
		dir := filepath.Dir(path)
		link, err = os.Readlink(path)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if filepath.IsAbs(link) {
			path = link
		} else {
			path = filepath.Join(dir, link)
		}
		links = append(links, path)
	}
	return links, nil
}
