package autoversion

import (
	"bufio"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

func gitTagsAutoVersion(autoVersion *manifest.AutoVersionBlock) (string, error) {
	versionRe, err := regexp.Compile(autoVersion.VersionPattern)
	if err != nil {
		return "", errors.WithStack(err)
	}
	if len(versionRe.SubexpNames()) != 2 {
		return "", errors.Errorf("%s: version pattern %s must have exactly one named capture group", autoVersion.GitTags, autoVersion.VersionPattern)
	}

	remoteURL := autoVersion.GitTags

	// --tags return all refs/tags/*
	// using --refs to remove duplicated tag lines ended with ^{}
	// output format of refs is
	// <oid> TAB <ref> LF
	// source: https://git-scm.com/docs/git-ls-remote
	out, err := exec.Command("git", "ls-remote", "--tags", "--refs", remoteURL).Output()
	if err != nil {
		return "", errors.Wrapf(err, "error listing tags for %s", remoteURL)
	}

	versions := make(manifest.Versions, 0)

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			return "", errors.Wrapf(err, "error parsing tags from line :%s for %s", line, remoteURL)
		}

		// clean up the prefix
		v := strings.ReplaceAll(parts[1], "refs/tags/", "")
		// use version patterns to find the valid version
		groups := versionRe.FindStringSubmatch(v)
		if len(groups) != 2 {
			// ignore invalid version according to config
			if autoVersion.IgnoreInvalidVersions {
				continue
			}
			return "", errors.Errorf("error parsing tags from line :%s for %s", line, remoteURL)
		}

		versions = append(versions, manifest.ParseVersion(groups[1]))
	}

	sort.Sort(versions)

	if len(versions) == 0 {
		return "", errors.Errorf("no tags found for %s", remoteURL)
	}

	return versions[len(versions)-1].String(), nil
}
