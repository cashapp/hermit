package util

import (
	"regexp"
	"strings"
)

const redactedPlaceholder = "****"

var urlCredentialsRe = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://)([^/?#@\s]+)@`)

// RedactCredentials masks userinfo in any URL appearing in s, so that credentials
// in hand-written source URLs are not written to logs, errors or the terminal.
func RedactCredentials(s string) string {
	return urlCredentialsRe.ReplaceAllStringFunc(s, func(match string) string {
		groups := urlCredentialsRe.FindStringSubmatch(match)
		scheme, userinfo := groups[1], groups[2]
		if i := strings.IndexByte(userinfo, ':'); i >= 0 {
			return scheme + userinfo[:i] + ":" + redactedPlaceholder + "@"
		}
		return scheme + redactedPlaceholder + "@"
	})
}
