package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
