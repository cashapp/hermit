package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGitParseRepo(t *testing.T) {
	repo, tag := parseGitURL("org-49461806@github.com:squareup/orc.git")
	require.Equal(t, "org-49461806@github.com:squareup/orc.git", repo)
	require.Equal(t, "", tag)
	repo, tag = parseGitURL("org-49461806@github.com:squareup/orc.git#v1.2.3")
	require.Equal(t, "org-49461806@github.com:squareup/orc.git", repo)
	require.Equal(t, "v1.2.3", tag)
}
