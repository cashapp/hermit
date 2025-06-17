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

// AutoVersionResult holds the complete result of auto-versioning including variables and checksums.
type AutoVersionResult struct {
	Version   string
	Variables map[string]string
	SHA256    string
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
		var result *AutoVersionResult
		switch {
		case block.autoVersion.GitHubRelease != "":
			latestVersion, err = gitHub(ghClient, block.autoVersion)
			if err == nil && latestVersion != "" {
				result = &AutoVersionResult{Version: latestVersion}
			}
		case block.autoVersion.HTML != nil:
			latestVersion, err = htmlAutoVersion(httpClient, block.autoVersion)
			if err == nil && latestVersion != "" {
				result = &AutoVersionResult{Version: latestVersion}
			}
		case block.autoVersion.JSON != nil:
			jsonResult, jsonErr := extractFromJSON(httpClient, block.autoVersion)
			err = jsonErr
			if err == nil && jsonResult != nil {
				latestVersion = jsonResult.Version
				result = &AutoVersionResult{
					Version:   jsonResult.Version,
					Variables: jsonResult.Variables,
					SHA256:    jsonResult.SHA256,
				}
			}
		case block.autoVersion.GitTags != "":
			latestVersion, err = gitTagsAutoVersion(block.autoVersion)
			if err == nil && latestVersion != "" {
				result = &AutoVersionResult{Version: latestVersion}
			}
		default:
			return "", errors.Errorf("%s: expected one of github-release, html, json, or git-tags", block.version.Pos)
		}
		if err != nil {
			return "", errors.Wrap(err, block.version.Pos.String())
		}

		// No version information found, skip.
		if result == nil || result.Version == "" {
			continue blocks
		}

		// No new version found, skip.
		for _, version := range block.version.Labels {
			if version == result.Version {
				continue blocks
			}
		}

		foundLatestVersion = true
		block.version.Labels = append(block.version.Labels, result.Version)

		// Write back variables and SHA256 for JSON auto-version
		if block.autoVersion.JSON != nil {
			writeBackJSONData(block.version, result)
		}
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

// writeBackJSONData writes variables and SHA256 from JSON auto-version extraction back to the version block.
func writeBackJSONData(versionBlock *hcl.Block, result *AutoVersionResult) {
	// Write variables if any were extracted
	if len(result.Variables) > 0 {
		upsertVarsInBlock(versionBlock, result.Variables)
	}

	// Write SHA256 if extracted
	if result.SHA256 != "" {
		upsertSHA256InBlock(versionBlock, result.SHA256)
	}

}

// upsertVarsInBlock adds or updates the vars map in a version block.
func upsertVarsInBlock(block *hcl.Block, variables map[string]string) {
	// Find existing vars entry
	var varsEntry *hcl.Entry
	for _, entry := range block.Body {
		if entry.Attribute != nil && entry.Attribute.Key == "vars" {
			varsEntry = entry
			break
		}
	}

	// Create vars map value
	varsMap := &hcl.Value{HaveMap: true}
	for key, value := range variables {
		varsMap.Map = append(varsMap.Map, &hcl.MapEntry{
			Key:   &hcl.Value{Str: &key},
			Value: &hcl.Value{Str: &value},
		})
	}

	if varsEntry == nil {
		// Create new vars entry
		varsEntry = &hcl.Entry{
			Attribute: &hcl.Attribute{
				Key:   "vars",
				Value: varsMap,
			},
		}
		block.Body = append(block.Body, varsEntry)
	} else {
		// Update existing vars - merge with existing variables
		if varsEntry.Attribute.Value.HaveMap {
			// Merge new variables with existing ones
			for key, value := range variables {
				// Check if variable already exists
				found := false
				for _, mapEntry := range varsEntry.Attribute.Value.Map {
					if mapEntry.Key.Str != nil && *mapEntry.Key.Str == key {
						mapEntry.Value = &hcl.Value{Str: &value}
						found = true
						break
					}
				}
				if !found {
					varsEntry.Attribute.Value.Map = append(varsEntry.Attribute.Value.Map, &hcl.MapEntry{
						Key:   &hcl.Value{Str: &key},
						Value: &hcl.Value{Str: &value},
					})
				}
			}
		} else {
			// Replace with new vars map
			varsEntry.Attribute.Value = varsMap
		}
	}
}

// upsertSHA256InBlock adds or updates the sha256 field in a version block.
func upsertSHA256InBlock(block *hcl.Block, sha256 string) {
	// Find existing sha256 entry
	var sha256Entry *hcl.Entry
	for _, entry := range block.Body {
		if entry.Attribute != nil && entry.Attribute.Key == "sha256" {
			sha256Entry = entry
			break
		}
	}

	if sha256Entry == nil {
		// Create new sha256 entry
		sha256Entry = &hcl.Entry{
			Attribute: &hcl.Attribute{
				Key:   "sha256",
				Value: &hcl.Value{Str: &sha256},
			},
		}
		block.Body = append(block.Body, sha256Entry)
	} else {
		// Update existing sha256
		sha256Entry.Attribute.Value = &hcl.Value{Str: &sha256}
	}
}
