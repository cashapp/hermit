package autoversion

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/alecthomas/hcl"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/github"
	hmanifest "github.com/cashapp/hermit/manifest"
)

// GitHubClient is the GitHub API subset that we need for auto-versioning.
type GitHubClient interface {
	LatestRelease(repo string) (*github.Release, error)
	Releases(repo string, limit int) (releases []*github.Release, err error)
}

type versionBlock struct {
	autoVersion *hmanifest.AutoVersionBlock
	version     *hcl.Block
}

// AutoVersion rewrites the given manifest with new version information if applicable.
//
// Auto-versioning configuration is defined in a "version > auto-version" block. If a new
// version is found in the defined location then the version block's versions are updated.
func AutoVersion(httpClient *http.Client, ghClient GitHubClient, path string) (latestVersion string, err error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", errors.WithStack(err)
	}
	ast, err := hcl.ParseBytes(content)
	if err != nil {
		return "", errors.WithStack(err)
	}
	blocks, err := parseVersionBlockFromManifest(ast)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse auto-version block(s)")
	} else if len(blocks) == 0 {
		return "", nil
	}

	foundLatestVersion := false
blocks:
	for _, block := range blocks {
		switch {
		case block.autoVersion.GitHubRelease != "":
			latestVersion, err = gitHub(ghClient, block.autoVersion)
		case block.autoVersion.HTML != nil:
			latestVersion, err = htmlAutoVersion(httpClient, block.autoVersion)
		default:
			return "", errors.Errorf("%s: expected either github-release or html", block.version.Pos)
		}
		if err != nil {
			return "", errors.Wrap(err, block.version.Pos.String())
		}

		// No version information found, skip.
		if latestVersion == "" {
			continue blocks
		}

		// No new version found, skip.
		for _, version := range block.version.Labels {
			if version == latestVersion {
				continue blocks
			}
		}

		foundLatestVersion = true
		block.version.Labels = append(block.version.Labels, latestVersion)
	}

	if !foundLatestVersion {
		return "", nil
	}

	// Update the manifest and write it out to disk.
	content, err = hcl.MarshalAST(ast)
	if err != nil {
		return "", errors.WithStack(err)
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
	return latestVersion, errors.WithStack(os.Rename(w.Name(), path))
}

// Parse the auto-version block from a manifest, if any.
//
// "versionBlock" will be empty is there are no auto-version blocks present.
func parseVersionBlockFromManifest(ast *hcl.AST) ([]versionBlock, error) {
	var blocks []versionBlock

	// Find auto-version info if any.
	err := hcl.Visit(ast, func(node hcl.Node, next func() error) error {
		if node, ok := node.(*hcl.Block); ok && node.Name == "auto-version" {
			autoVersion := &hmanifest.AutoVersionBlock{}
			if err := hcl.UnmarshalBlock(node, autoVersion); err != nil {
				return errors.WithStack(err)
			}
			autoVersionedBlock := node.Parent.(*hcl.Entry).Parent.(*hcl.Block)

			blocks = append(blocks, versionBlock{autoVersion, autoVersionedBlock})
		}
		return next()
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return blocks, nil
}
