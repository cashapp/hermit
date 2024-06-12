package util

import (
	"os"
	"strings"

	"github.com/cashapp/hermit/errors"
)

// FilePatcher is used to update or set lines in a text file,
// separated by given start and end line markers
type FilePatcher struct {
	startLine string
	endLine   string
	fileMode  os.FileMode
}

// NewFilePatcher creates a new FilePatcher
func NewFilePatcher(start, end string) *FilePatcher {
	return &FilePatcher{start, end, 0644}
}

// Patch updates the contents in a file.
func (fp *FilePatcher) Patch(fileName, content string) (bool, error) {
	file, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, fp.fileMode)
	if err != nil {
		return false, errors.WithStack(err)
	}
	defer file.Close() // nolint: gosec

	previousBts, err := os.ReadFile(fileName)
	if err != nil {
		return false, errors.WithStack(err)
	}
	previous := string(previousBts)
	from := strings.Index(previous, fp.startLine+"\n")
	to := strings.Index(previous, fp.endLine) - 1

	result := ""
	if from >= 0 && to >= 0 {
		result = previous[:(from+len(fp.startLine)+1)] + content + previous[to:]
	} else if from >= 0 {
		return false, errors.Errorf("found '%s' without '%s' in %s", fileName, fp.startLine, fp.endLine)
	} else if to >= 0 {
		return false, errors.Errorf("found '%s' without '%s' in %s", fileName, fp.endLine, fp.startLine)
	} else {
		result = strings.Join([]string{previous, fp.startLine, content, fp.endLine}, "\n") + "\n"
	}

	changed := result != previous
	if !changed {
		return false, nil
	}

	err = os.WriteFile(fileName, []byte(result), 0600)
	if err != nil {
		return false, errors.WithStack(err)
	}

	return true, nil
}
