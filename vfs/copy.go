package vfs

import (
	"io"
	"io/fs"
	"os"

	"github.com/pkg/errors"
)

// CopyFile from srcFS/src to dst.
func CopyFile(srcFS fs.FS, src string, dst string) (reterr error) {
	source, err := srcFS.Open(src)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		err := source.Close()
		if err != nil {
			reterr = err
		}
	}()

	destination, err := os.Create(dst)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		err := destination.Close()
		if err != nil {
			reterr = err
		}
	}()
	_, err = io.Copy(destination, source)
	return errors.WithStack(err)
}
