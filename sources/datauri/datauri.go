package datauri

import (
	"encoding/base64"
	"strings"

	"github.com/cashapp/hermit/errors"
)

// Encoding using the [Data URI scheme](https://en.wikipedia.org/wiki/Data_URI_scheme)
// See [RFC2397](https://www.rfc-editor.org/rfc/rfc2397) for additional information.
const contentType = "application/json"
const encoding = "base64"
const prefix = "data:" + contentType + ";" + encoding + ","

// Encode takes a `[]byte` content and generates a valid data URI.
// This URI can be put into a browser to see the contents.
func Encode(content []byte) string {
	return prefix + base64.StdEncoding.EncodeToString(content)
}

var doesNotHavePrefixErr = errors.Errorf("Only data uris with json-type, base64-encoded are valid. URI must have prefix \"%s\"", prefix)

// Decode takes a data URI and extracts the `[]byte` contents.
func Decode(uri string) ([]byte, error) {
	if !strings.HasPrefix(uri, prefix) {
		//nolint: wrapcheck
		return nil, doesNotHavePrefixErr
	}

	decoded, err := base64.StdEncoding.DecodeString(uri[len(prefix):])
	if err != nil {
		return nil, errors.Wrap(err, "error decoding data uri")
	}
	return decoded, nil
}
