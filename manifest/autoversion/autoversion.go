package autoversion

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cashapp/hermit/manifest/manifestutils"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"

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
func AutoVersion(httpClient *http.Client, ghClient GitHubClient, path string, state *state.State, l *ui.UI) (latestVersion string, err error) {
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

	// prepare for PopulateDigestsForVersion
	var absolute string
	absolute, err = filepath.Abs(path)
	if err != nil {
		return "", errors.WithStack(err)
	}
	dir := filepath.Dir(absolute)
	if err != nil {
		return "", errors.WithStack(err)
	}
	annotated := &hmanifest.AnnotatedManifest{
		FS:   os.DirFS(dir),
		Name: strings.Replace(filepath.Base(path), ".hcl", "", 1),
		Path: absolute,
	}

	annotated, err = hmanifest.LoadManifestBytes(content, annotated)
	if err != nil {
		return "", errors.WithStack(err)
	}
	version := hmanifest.VersionBlock{
		Version: []string{latestVersion},
	}
	var s, d []string

	s, d, err = manifestutils.PopulateDigestsForVersion(l, state, annotated, &version)
	if err != nil {
		return "", errors.WithStack(err)
	}

	err = populateDigestInAst(ast, s, d)
	if err != nil {
		return "", errors.WithStack(err)
	}

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

// Helper function to generate a usable hcl.MapEntry
func getMapEntry(source string, digest string) (*hcl.MapEntry, error) {
	hclVal, err := hcl.ParseString(fmt.Sprintf("sha256sums = {\"%s\": \"%s\"}", source, digest))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// As the hcl is hardcoded above using hardcoded indexes is safe.
	return hclVal.Entries[0].Attribute.Value.Map[0], nil
}

// add the digest values into the existing AST.
func populateDigestInAst(ast *hcl.AST, source []string, digest []string) error {
	found := false
	// make sure the arguments are as expected
	if source == nil || digest == nil || len(source) == 0 || len(source) != len(digest) {
		return errors.New("Source and Digest arrays are not as expected")
	}
	// If the input manifest already has sha256sums
	for _, v := range ast.Entries {
		if v.Attribute != nil && v.Attribute.Key != "" {
			if v.Attribute.Key == "sha256sums" {
				for i, val := range source {
					me, err := getMapEntry(val, digest[i])
					if err != nil {
						return errors.WithStack(err)
					}
					v.Attribute.Value.Map = append(v.Attribute.Value.Map, me)
				}
				found = true
				break
			}
		}
	}

	// If the input manifest does not have sha256sums then make a new one.
	if !found {
		hclString := "sha256sums = {"
		for i, val := range source {
			hclString += fmt.Sprintf("\"%s\": \"%s\"", val, digest[i])
			if i < len(source) {
				hclString += ","
			}
		}
		hclString += "}"
		hclVal, err := hcl.ParseString(hclString)
		if err != nil {
			return errors.WithStack(err)

		}
		ast.Entries = append(ast.Entries, hclVal.Entries[0])
	}
	return nil
}
