package util

import (
	"fmt"
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

// GitConfigEnv passes git config via the environment (git >= 2.31), keeping
// secrets out of process arguments and out of the repository's .git/config.
func GitConfigEnv(pairs ...string) []string {
	if len(pairs)%2 != 0 {
		panic("GitConfigEnv requires an even number of arguments")
	}
	env := []string{fmt.Sprintf("GIT_CONFIG_COUNT=%d", len(pairs)/2)}
	for i := 0; i < len(pairs); i += 2 {
		env = append(env,
			fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", i/2, pairs[i]),
			fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", i/2, pairs[i+1]))
	}
	return env
}

// ValidateGitURL rejects URLs selecting a transport Hermit does not support, and
// URLs that git would interpret as an option.
func ValidateGitURL(url string) error {
	if strings.HasPrefix(url, "-") {
		return errors.Errorf("invalid git URL %q: cannot start with '-'", url)
	}
	scheme := gitURLSchemeRe.FindString(url)
	switch rest := url[len(scheme):]; {
	case strings.HasPrefix(rest, "://"):
		if !slices.Contains(allowedGitSchemes, strings.ToLower(scheme)) {
			return errors.Errorf("invalid git URL %q: scheme must be one of %s", url, strings.Join(allowedGitSchemes, ", "))
		}

	case strings.HasPrefix(rest, "::"):
		return errors.Errorf("invalid git URL %q: remote helpers are not supported", url)
	}
	return nil
}
