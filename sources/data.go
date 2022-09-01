package sources

import (
	"encoding/json"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/sources/datauri"
)

// NewDataSource parsed the uri as a [Data URI scheme](https://en.wikipedia.org/wiki/Data_URI_scheme).
// See [RFC2397](https://www.rfc-editor.org/rfc/rfc2397) for additional information on the scheme.
// The only valid content type is "application/json", the only valid encoding is "base64", &
// it's assumed that the content encoded is a json map of binary name to file contents.
func NewDataSource(uri string) (MemSource, error) {
	decoded, err := datauri.Decode(uri)
	if err != nil {
		return nil, errors.Wrap(err, "unable to decode the data uri")
	}

	sources := map[string]string{}
	if err := json.Unmarshal(decoded, &sources); err != nil {
		return nil, errors.Wrap(err, "unable to unmarshall the json")
	}
	return NewMemSources(sources), nil
}
