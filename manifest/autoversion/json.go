package autoversion

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"

	"github.com/itchyny/gojq"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
)

func jsonAutoVersion(client *http.Client, autoVersion *manifest.AutoVersionBlock) (string, error) {
	versionRe, err := regexp.Compile(autoVersion.VersionPattern)
	if err != nil {
		return "", errors.WithStack(err)
	}

	url := autoVersion.JSON.URL
	resp, err := client.Get(url) // nolint
	if err != nil {
		return "", errors.Wrapf(err, "could not retrieve auto-version information")
	}
	defer resp.Body.Close()

	var data any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", errors.Wrapf(err, "%s: could not parse JSON", url)
	}

	query, err := gojq.Parse(autoVersion.JSON.JQ)
	if err != nil {
		return "", errors.Wrapf(err, "could not parse jq expression %q", autoVersion.JSON.JQ)
	}

	var candidates []string
	iter := query.Run(data)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return "", errors.Wrapf(err, "jq expression %q failed", autoVersion.JSON.JQ)
		}
		candidates = append(candidates, fmt.Sprintf("%v", v))
	}

	versions := make(manifest.Versions, 0, len(candidates))
	for _, value := range candidates {
		groups := versionRe.FindStringSubmatch(value)
		if groups == nil {
			if autoVersion.IgnoreInvalidVersions {
				continue
			}
			return "", errors.Errorf("version must match the pattern %s but is %s", autoVersion.VersionPattern, value)
		}
		versions = append(versions, manifest.ParseVersion(groups[1]))
	}
	sort.Sort(versions)

	if len(versions) == 0 {
		return "", errors.Errorf("no versions matched on %s", url)
	}

	return versions[len(versions)-1].String(), nil
}
