package autoversion

import (
	"regexp"

	"github.com/cashapp/hermit/errors"
	hmanifest "github.com/cashapp/hermit/manifest"
)

func gitHub(client GitHubClient, autoVersion *hmanifest.AutoVersionBlock) (string, error) {
	release, err := client.LatestRelease(autoVersion.GitHubRelease)
	if err != nil {
		return "", errors.WithStack(err)
	}
	versionRe, err := regexp.Compile(autoVersion.VersionPattern)
	if err != nil {
		return "", errors.WithStack(err)
	}
	groups := versionRe.FindStringSubmatch(release.TagName)
	if groups == nil {
		if autoVersion.IgnoreInvalidVersions {
			return "", nil
		}
		return "", errors.Errorf("%s: latest release must match the pattern %s but is %s", autoVersion.GitHubRelease, autoVersion.VersionPattern, release.TagName)
	}
	latestVersion := groups[1]
	return latestVersion, nil
}
