package cache

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/pkg/errors"
)

// Download a file with resumption.
//
// On return "w" will be either at the beginning of the file, or at the resumption point.
//
// response.ContentLength will be the number of bytes remaining.
func Download(client *http.Client, uri, file string) (w *os.File, response *http.Response, err error) {
	if client == nil {
		client = http.DefaultClient
	}
	w, err = os.OpenFile(file, os.O_RDWR|os.O_CREATE, 0666) // nolint: gosec
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	defer closeIfError(&err, w)
	info, err := os.Stat(file)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, errors.WithStack(err)
	}
	resumed := info.Size()
	req, err := http.NewRequest("GET", uri, nil) // nolint: noctx
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	// req.Header.Set("If-Range", info.ModTime().UTC().Format("Mon, 2 Jan 2006 15:04:05 GMT"))
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumed))
	response, err = client.Do(req)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	defer closeIfError(&err, response.Body)
	switch response.StatusCode {
	case http.StatusPartialContent:
		_, err = w.Seek(0, 2)
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
	default:
		// Anything else, assume we need to start again.
		err = w.Truncate(0)
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
	}
	return w, response, nil
}

func closeIfError(err *error, c io.Closer) {
	if *err != nil {
		_ = c.Close()
	}
}
