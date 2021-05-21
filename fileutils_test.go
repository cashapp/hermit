package hermit_test

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type TestTarGz struct {
	files map[string]string
}

// StringFileInfo provides os.FileInfo implementation
type StringFileInfo struct {
	name string
	data string
}

func (targz *TestTarGz) Write(t *testing.T, w io.Writer) {
	t.Helper()
	gw := gzip.NewWriter(w)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	for name, data := range targz.files {
		header, _ := tar.FileInfoHeader(&StringFileInfo{name, data}, "")

		err := tw.WriteHeader(header)
		require.NoError(t, err)

		_, err = tw.Write([]byte(data))
		require.NoError(t, err)
	}
}

func (info *StringFileInfo) Name() string {
	return info.name
}

func (info *StringFileInfo) Size() int64 {
	return int64(len([]byte(info.data)))
}

func (info *StringFileInfo) Mode() os.FileMode {
	return os.ModePerm
}

func (info *StringFileInfo) ModTime() time.Time {
	return time.Now()
}

func (info *StringFileInfo) IsDir() bool {
	return false
}

func (info *StringFileInfo) Sys() interface{} {
	return nil
}
