// Package redact masks credentials in values that have crossed an output boundary.
package redact

import (
	"regexp"
	"strings"
)

const placeholder = "****"

var urlUserinfoRE = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)([^/?#\s]+)@`)

// Credentials masks URL credentials in value.
func Credentials(value string) string {
	return urlUserinfoRE.ReplaceAllStringFunc(value, func(match string) string {
		parts := urlUserinfoRE.FindStringSubmatch(match)
		scheme, userinfo := parts[1], parts[2]
		if i := strings.IndexByte(userinfo, ':'); i >= 0 {
			return scheme + userinfo[:i] + ":" + placeholder + "@"
		}
		// A username-only HTTP URL is how tokens are commonly supplied. Other
		// schemes commonly use non-secret usernames, such as ssh://git@host.
		switch strings.ToLower(strings.TrimSuffix(scheme, "://")) {
		case "http", "https":
			return scheme + placeholder + "@"
		default:
			return match
		}
	})
}
