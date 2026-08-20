package util

import (
	"regexp"
	"slices"
	"strings"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/redact"
)

var (
	allowedGitSchemes = []string{"file", "git", "http", "https", "ssh"}
	gitURLSchemeRe    = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*`)
)

// GitArgs pins git's transport policy, as git otherwise resolves an unrecognised
// transport by executing git-remote-<transport> from Hermit's own bin/ on PATH.
func GitArgs(args ...string) []string {
	out := make([]string, 0, 3+2*len(allowedGitSchemes)+len(args))
	out = append(out, "git", "-c", "protocol.allow=never")
	for _, scheme := range allowedGitSchemes {
		out = append(out, "-c", "protocol."+scheme+".allow=always")
	}
	return append(out, args...)
}

// ValidateGitURL rejects URLs selecting a transport Hermit does not support, and
// URLs that git would interpret as an option.
func ValidateGitURL(url redact.URL) error {
	raw := url.Reveal()
	if strings.HasPrefix(raw, "-") {
		return errors.Errorf("invalid git URL %q: cannot start with '-'", url)
	}
	scheme := gitURLSchemeRe.FindString(raw)
	switch rest := raw[len(scheme):]; {
	case strings.HasPrefix(rest, "://"):
		if !slices.Contains(allowedGitSchemes, strings.ToLower(scheme)) {
			return errors.Errorf("invalid git URL %q: scheme must be one of %s", url, strings.Join(allowedGitSchemes, ", "))
		}

	case strings.HasPrefix(rest, "::"):
		return errors.Errorf("invalid git URL %q: remote helpers are not supported", url)
	}
	return nil
}
