package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"

	"github.com/cashapp/hermit/errors"
)

// Hash computes a (probably) unique hash of "values".
func Hash(values ...interface{}) string {
	w := sha256.New()
	enc := json.NewEncoder(w)
	for _, value := range values {
		_ = enc.Encode(value)
	}
	return hex.EncodeToString(w.Sum(nil))
}

// Sha256LocalFile Utility function to hash a downloaded file.
func Sha256LocalFile(path string) (string, error) {
	r, err := os.Open(path)
	if err != nil {
		return "", errors.WithStack(err)
	}
	h := sha256.New()
	_, err = io.Copy(h, r)
	if err != nil {
		r.Close()
		return "", errors.WithStack(err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
