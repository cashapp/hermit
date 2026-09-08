package util

import "net/url"

// StripURLError unwraps *url.Error so stdlib messages cannot leak credentials
// embedded in the URL; callers reattach the redacted URL.
func StripURLError(err error) error {
	if uerr, ok := err.(*url.Error); ok { //nolint:errorlint
		return uerr.Err
	}
	return err
}
