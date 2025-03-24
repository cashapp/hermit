package github

import (
	"github.com/cashapp/hermit/errors"
	"github.com/gobwas/glob"
)

// RepoMatcher is used to determine which repositories will use authenticated requests.
type RepoMatcher func(owner, repo string) bool

// GlobRepoMatcher accepts a list of glob patterns and returns a [RepoMatcher]
// that will match 'owner/repo' pairs against the globs.
//
// It returns an error if any of the patterns are invalid.
func GlobRepoMatcher(patterns []string) (RepoMatcher, error) {
	globs := make([]glob.Glob, len(patterns))
	for i, pattern := range patterns {
		g, err := glob.Compile(pattern)
		if err != nil {
			return nil, errors.Errorf("bad pattern [%d] %q: %v", i, pattern, err)
		}
		globs[i] = g
	}

	return func(owner, repo string) bool {
		ownerRepo := owner + "/" + repo
		for _, g := range globs {
			if g.Match(ownerRepo) {
				return true
			}
		}
		return false
	}, nil
}
