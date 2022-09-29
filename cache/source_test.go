package cache

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestGitParseRepo(t *testing.T) {
	repo, tag := parseGitURL("org-49461806@github.com:squareup/orc.git")
	assert.Equal(t, "org-49461806@github.com:squareup/orc.git", repo)
	assert.Equal(t, "", tag)
	repo, tag = parseGitURL("org-49461806@github.com:squareup/orc.git#v1.2.3")
	assert.Equal(t, "org-49461806@github.com:squareup/orc.git", repo)
	assert.Equal(t, "v1.2.3", tag)
}
