package sources

import "github.com/cashapp/hermit/internal/redact"

// SourceURI is a source URI. Its String method is safe for output; raw access is
// deliberately explicit through Get.
type SourceURI struct {
	value string
}

// NewSourceURI wraps a raw source URI.
func NewSourceURI(value string) SourceURI {
	return SourceURI{value: value}
}

// Get returns the raw source URI for operations that require it.
func (s SourceURI) Get() string {
	return s.value
}

// String returns the source URI with credentials redacted.
func (s SourceURI) String() string {
	return redact.Credentials(s.value)
}
