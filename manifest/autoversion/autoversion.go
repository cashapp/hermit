package autoversion

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/alecthomas/hcl"
	"github.com/pkg/errors"

	"github.com/cashapp/hermit/github"
	hmanifest "github.com/cashapp/hermit/manifest"
)

// GitHubClient is the GitHub API subset that we need for auto-versioning.
type GitHubClient interface {
	LatestRelease(repo string) (*github.Release, error)
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
	ast, hclBlock, autoVersionBlock, err := parseVersionBlockFromManifest(content)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse auto-version block")
	}
	switch {
	case autoVersionBlock.GitHubRelease != "":
		latestVersion, err = gitHub(ghClient, autoVersionBlock)
	case autoVersionBlock.HTML != nil:
		latestVersion, err = htmlAutoVersion(httpClient, autoVersionBlock)
	default:
		return "", errors.Errorf("%s: expected either github-release or html", hclBlock.Pos)
	}
	if err != nil {
		return "", errors.Wrap(err, hclBlock.Pos.String())
	}
	// No version information found, skip.
	if latestVersion == "" {
		return "", nil
	}

	// No new version found, skip.
	for _, version := range hclBlock.Labels {
		if version == latestVersion {
			return "", nil
		}
	}

	// Update the manifest and write it out to disk.
	hclBlock.Labels = append(hclBlock.Labels, latestVersion)
	content, err = hcl.MarshalAST(ast)
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
// "hclBlock" and "autoVersionBlock" will be nil if there is no auto-version block present.
//
// "hclBlock" is the block containing the auto-version information. Its "Labels" field should be updated with the new versions, if any.
func parseVersionBlockFromManifest(manifest []byte) (ast *hcl.AST, hclBlock *hcl.Block, autoVersionBlock *hmanifest.AutoVersionBlock, err error) {
	ast, err = hcl.ParseBytes(manifest)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}
	var (
		autoVersion        *hmanifest.AutoVersionBlock
		autoVersionedBlock *hcl.Block
	)
	// Find auto-version info if any.
	err = hcl.Visit(ast, func(node hcl.Node, next func() error) error {
		if node, ok := node.(*hcl.Block); ok && node.Name == "auto-version" {
			autoVersion = &hmanifest.AutoVersionBlock{}
			err = hcl.UnmarshalBlock(node, autoVersion)
			if err != nil {
				return errors.WithStack(err)
			}
			autoVersionedBlock = node.Parent.(*hcl.Entry).Parent.(*hcl.Block)
		}
		return next()
	})
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}
	return ast, autoVersionedBlock, autoVersion, nil
}
