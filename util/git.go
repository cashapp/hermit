package util

import (
	"regexp"
	"slices"
	"strings"

	"github.com/cashapp/hermit/errors"
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

type sourceValue interface {
	Get() string
	String() string
}

// ValidateGitURL rejects sources selecting a transport Hermit does not support,
// and sources that git would interpret as an option.
func ValidateGitURL(source sourceValue) error {
	url := source.Get()
	if strings.HasPrefix(url, "-") {
		return errors.Errorf("invalid git URL %q: cannot start with '-'", source)
	}
	scheme := gitURLSchemeRe.FindString(url)
	switch rest := url[len(scheme):]; {
	case strings.HasPrefix(rest, "://"):
		if !slices.Contains(allowedGitSchemes, strings.ToLower(scheme)) {
			return errors.Errorf("invalid git URL %q: scheme must be one of %s", source, strings.Join(allowedGitSchemes, ", "))
		}

	case strings.HasPrefix(rest, "::"):
		return errors.Errorf("invalid git URL %q: remote helpers are not supported", source)
	}
	return nil
}
