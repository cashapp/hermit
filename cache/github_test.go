package cache

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestGlobRepoMatcher(t *testing.T) {
	type ownerRepo struct{ owner, repo string }

	tests := []struct {
		name     string
		patterns []string

		expectMatches    []ownerRepo
		expectNotMatches []ownerRepo
	}{
		{
			name:     "all",
			patterns: []string{"*"},
			expectMatches: []ownerRepo{
				{"example", "repo"},
				{"foo", "bar"},
				{"baz", "qux"},
			},
			// nothing not matched
		},
		{
			name: "org",
			patterns: []string{
				"example/*",
			},
			expectMatches: []ownerRepo{
				{"example", "repo"},
				{"example", "bar"},
			},
			expectNotMatches: []ownerRepo{
				{"foo", "bar"},
				{"examplesuffix", "repo"},
				{"prefixexample", "repo"},
			},
		},
		{
			name:     "partial org",
			patterns: []string{"example-*/*"},
			expectMatches: []ownerRepo{
				{"example-foo", "repo"},
				{"example-bar", "repo"},
			},
			expectNotMatches: []ownerRepo{
				{"example", "repo"},
			},
		},
		{
			name: "multiple",
			patterns: []string{
				"example/*",
				"*/homebrew-*",
			},
			expectMatches: []ownerRepo{
				{"example", "repo"},
				{"foo", "homebrew-tap"},
				{"homebrew", "homebrew-tap"},
			},
			expectNotMatches: []ownerRepo{
				{"foo", "bar"},
				{"foo", "homebrew"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := GlobRepoMatcher(tt.patterns)
			assert.NoError(t, err)

			for _, match := range tt.expectMatches {
				t.Run(match.owner+"/"+match.repo, func(t *testing.T) {
					assert.True(t, matcher(match.owner, match.repo))
				})
			}

			for _, match := range tt.expectNotMatches {
				t.Run(match.owner+"/"+match.repo, func(t *testing.T) {
					assert.False(t, matcher(match.owner, match.repo))
				})
			}
		})
	}
}

func TestGlobRepoMatcherBadPattern(t *testing.T) {
	_, err := GlobRepoMatcher([]string{"["})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `bad pattern [0]`)
}
