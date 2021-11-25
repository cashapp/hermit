package manifest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cashapp/hermit/github"
)

type testGHAPI struct{}

func (v testGHAPI) LatestRelease(repo string) (*github.Release, error) {
	return &github.Release{TagName: "v3.2.150"}, nil
}

func TestGitHubAutoVersion(t *testing.T) {
	source := `
description = "Jenkins X CLI"
test = "jx version"
binaries = ["jx"]

linux {
  source = "https://github.com/jenkins-x/jx/releases/download/v${version}/jx-linux-amd64.tar.gz"
}
darwin {
  source = "https://github.com/jenkins-x/jx/releases/download/v${version}/jx-darwin-amd64.tar.gz"
}

version "3.2.137" "3.2.140" {
  auto-version {
    github-release = "jenkins-x/jx"
  }
}

channel "stable" {
  update = "24h"
  version = "3.*"
}
`
	latestVersion, actual, err := autoVersion(testGHAPI{}, []byte(source))
	require.NoError(t, err)
	require.Equal(t, "3.2.150", latestVersion)
	expected := `description = "Jenkins X CLI"
test = "jx version"
binaries = ["jx"]

linux {
  source = "https://github.com/jenkins-x/jx/releases/download/v${version}/jx-linux-amd64.tar.gz"
}

darwin {
  source = "https://github.com/jenkins-x/jx/releases/download/v${version}/jx-darwin-amd64.tar.gz"
}

version "3.2.137" "3.2.140" "3.2.150" {
  auto-version {
    github-release = "jenkins-x/jx"
  }
}

channel "stable" {
  update = "24h"
  version = "3.*"
}
`
	require.Equal(t, expected, string(actual))
}
