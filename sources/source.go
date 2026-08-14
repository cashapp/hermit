package sources

import "github.com/cashapp/hermit/internal/redact"

// Source is a source URI. Its String method is safe for output; raw access is
// deliberately explicit through Get.
type Source struct {
	value string
}

// NewSource wraps a raw source URI.
func NewSource(value string) Source {
	return Source{value: value}
}

// Get returns the raw source URI for operations that require it.
func (s Source) Get() string {
	return s.value
}

// String returns the source URI with credentials redacted.
func (s Source) String() string {
	return redact.Credentials(s.value)
}
