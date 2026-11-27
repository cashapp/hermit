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

	// gitLocalEnvVars are the variables "git rev-parse --local-env-vars" lists:
	// the ones that tie a git process to one particular repository. Git sets
	// several of them for hooks (GIT_DIR and GIT_INDEX_FILE in a linked
	// worktree, for example), so a Hermit activated from a hook inherits them.
	//
	// The list is kept here rather than queried at run time so that building
	// a git command never needs an extra git process, and git rarely adds to
	// it. A test checks it against the installed git.
	//
	// GIT_CONFIG_PARAMETERS and GIT_CONFIG_COUNT are left out: they carry
	// "git -c" settings, not repository identity, and git itself keeps them
	// when it runs a command in another repository (a submodule).
	gitLocalEnvVars = map[string]bool{
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
		"GIT_COMMON_DIR":                   true,
		"GIT_CONFIG":                       true,
		"GIT_DIR":                          true,
		"GIT_GRAFT_FILE":                   true,
		"GIT_IMPLICIT_WORK_TREE":           true,
		"GIT_INDEX_FILE":                   true,
		"GIT_NO_REPLACE_OBJECTS":           true,
		"GIT_OBJECT_DIRECTORY":             true,
		"GIT_PREFIX":                       true,
		"GIT_REPLACE_REF_BASE":             true,
		"GIT_SHALLOW_FILE":                 true,
		"GIT_WORK_TREE":                    true,
	}
)

// WithoutGitLocalEnv returns environ (in os.Environ form) without git's
// repository-local variables. Git commands that act on Hermit's own
// repositories, such as its package source caches, run with this environment
// so that they cannot act on a repository the caller's environment points at.
func WithoutGitLocalEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if !gitLocalEnvVars[name] {
			out = append(out, kv)
		}
	}
	return out
}

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
