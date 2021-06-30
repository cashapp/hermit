package manifest

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/alecthomas/hcl"
	"github.com/pkg/errors"
)

// AutoVersion rewrites the given manifest with new version information if applicable.
//
// Auto-versioning configuration is defined in a "version > auto-version" block. If a new
// version is found in the defined location then the version block's versions are updated.
func AutoVersion(path string) (version string, err error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", errors.WithStack(err)
	}
	version, content, err = autoVersion(content, realVersioner{})
	if err != nil {
		return "", errors.Wrap(err, path)
	}
	if content == nil || version == "" {
		return "", nil
	}
	w, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer w.Close() // nolint
	defer os.Remove(w.Name())
	_, err = w.Write(content)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return version, errors.WithStack(os.Rename(w.Name(), path))
}

type versioner interface {
	latestGitHubRelease(repo string) (string, error)
}

func autoVersion(manifest []byte, versioner versioner) (string, []byte, error) {
	ast, err := hcl.ParseBytes(manifest)
	if err != nil {
		return "", nil, errors.WithStack(err)
	}
	var (
		autoVersion        *AutoVersionBlock
		autoVersionedBlock *hcl.Block
	)
	// Find auto-version info if any.
	err = hcl.Visit(ast, func(node hcl.Node, next func() error) error {
		if node, ok := node.(*hcl.Block); ok && node.Name == "auto-version" {
			autoVersion = &AutoVersionBlock{}
			err = hcl.UnmarshalBlock(node, autoVersion)
			if err != nil {
				return errors.WithStack(err)
			}
			autoVersionedBlock = node.Parent.(*hcl.Entry).Parent.(*hcl.Block)
		}
		return next()
	})
	if err != nil {
		return "", nil, errors.WithStack(err)
	}
	if autoVersion == nil {
		return "", nil, nil
	}
	latestVersion, err := versioner.latestGitHubRelease(autoVersion.GitHubRelease)
	if err != nil {
		return "", nil, errors.WithStack(err)
	}
	versionRe, err := regexp.Compile(autoVersion.VersionPattern)
	if err != nil {
		return "", nil, errors.WithStack(err)
	}
	groups := versionRe.FindStringSubmatch(latestVersion)
	if groups == nil {
		return "", nil, errors.Errorf("%s: latest release must match the pattern %s but is %s", autoVersion.GitHubRelease, autoVersion.VersionPattern, latestVersion)
	}
	latestVersion = groups[1]
	// Check if version already exists.
	for _, label := range autoVersionedBlock.Labels {
		if label == latestVersion {
			return "", nil, nil
		}
	}
	autoVersionedBlock.Labels = append(autoVersionedBlock.Labels, latestVersion)
	content, err := hcl.MarshalAST(ast)
	return latestVersion, content, errors.WithStack(err)
}

type gitHubLatestReleaseResponse struct {
	TagName string `json:"tag_name"`
}

type realVersioner struct{}

func (realVersioner) latestGitHubRelease(repo string) (string, error) {
	resp, err := http.Get("https://api.github.com/repos/" + repo + "/releases/latest") // nolint
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer resp.Body.Close()
	ghResp := gitHubLatestReleaseResponse{}
	err = json.NewDecoder(resp.Body).Decode(&ghResp)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return ghResp.TagName, nil
}
