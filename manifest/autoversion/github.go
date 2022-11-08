package autoversion

import (
	"regexp"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/github"
	hmanifest "github.com/cashapp/hermit/manifest"
)

func gitHub(client GitHubClient, autoVersion *hmanifest.AutoVersionBlock) (string, error) {
	var (
		releases []*github.Release
		err      error
	)
	const history = 100
	// When there may be invalid versions, we need to fetch multiple release to find the latest valid one.
	// When IgnoreInvalidVersions is off, fetch only the latest release for performance.
	if autoVersion.IgnoreInvalidVersions {
		releases, err = client.Releases(autoVersion.GitHubRelease, history)
		if err != nil {
			return "", errors.WithStack(err)
		}
	} else {
		release, err := client.LatestRelease(autoVersion.GitHubRelease)
		if err != nil {
			return "", errors.WithStack(err)
		}
		releases = []*github.Release{release}
	}
	versionRe, err := regexp.Compile(autoVersion.VersionPattern)
	if err != nil {
		return "", errors.WithStack(err)
	}
	if len(versionRe.SubexpNames()) != 2 {
		return "", errors.Errorf("%s: version pattern %s must have exactly one named capture group", autoVersion.GitHubRelease, autoVersion.VersionPattern)
	}
	for _, release := range releases {
		groups := versionRe.FindStringSubmatch(release.TagName)
		if len(groups) == 2 {
			latestVersion := groups[1]
			return latestVersion, nil
		}
		if !autoVersion.IgnoreInvalidVersions {
			return "", errors.Errorf("%s: latest release must match the pattern %s but is %s", autoVersion.GitHubRelease, autoVersion.VersionPattern, release.TagName)
		}
	}
	return "", errors.Errorf("%s: did not find a release matching the pattern %s in the last %d releases", autoVersion.GitHubRelease, autoVersion.VersionPattern, history)
}
