package autoversion

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/alecthomas/hcl"
	"github.com/tidwall/gjson"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/manifest"
)

// GitHubClient is the GitHub API subset that we need for auto-versioning.
type GitHubClient interface {
	LatestRelease(repo string) (*github.Release, error)
	Releases(repo string, limit int) (releases []*github.Release, err error)
}

type versionBlock struct {
	autoVersion *manifest.AutoVersionBlock
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
			// New extract block approach only
			latestVersion, err = extractVersionFromExtractBlock(httpClient, block.autoVersion)
			if err == nil && latestVersion != "" {
				result = &AutoVersionResult{Version: latestVersion}
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

		// Add new version label (consistent with other auto-version types)
		block.version.Labels = append(block.version.Labels, result.Version)

		// For JSON auto-version, extract variables and write vars cache to auto-version block
		if block.autoVersion.JSON != nil {
			// Extract all variables from the extract block
			versionData, err := extractJSONVariablesFromExtractBlock(httpClient, block.autoVersion.JSON, result.Version)
			if err != nil {
				return "", errors.Wrap(err, block.version.Pos.String())
			}

			// Write vars cache to the auto-version block in AST
			writeAutoVersionVarsCache(block.version, result.Version, versionData)
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
			autoVersion := &manifest.AutoVersionBlock{}
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

// extractVersionFromExtractBlock extracts the version using the extract.version path
func extractVersionFromExtractBlock(client *http.Client, autoVersion *manifest.AutoVersionBlock) (string, error) {
	if autoVersion.JSON == nil || autoVersion.JSON.Extract == nil {
		return "", errors.New("extract block not found in JSON auto-version configuration")
	}

	// Fetch JSON data
	req, err := http.NewRequestWithContext(context.Background(), "GET", autoVersion.JSON.URL, nil)
	if err != nil {
		return "", errors.Wrapf(err, "could not create request for auto-version information")
	}

	// Add custom headers
	for key, value := range autoVersion.JSON.Headers {
		req.Header.Set(key, value)
	}

	// Set default Accept header if not specified
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", errors.Wrapf(err, "could not retrieve auto-version information")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "could not read response body")
	}

	// Parse JSON and extract version
	if !gjson.ValidBytes(body) {
		return "", errors.Errorf("invalid JSON response")
	}

	result := gjson.GetBytes(body, autoVersion.JSON.Extract.Version)
	if !result.Exists() {
		return "", errors.Errorf("version path %q matched no results", autoVersion.JSON.Extract.Version)
	}

	version := result.String()

	// Apply version pattern if specified
	if autoVersion.VersionPattern != "" {
		versionRe, err := regexp.Compile(autoVersion.VersionPattern)
		if err != nil {
			return "", errors.WithStack(err)
		}
		groups := versionRe.FindStringSubmatch(version)
		if groups == nil {
			return "", errors.Errorf("version %q does not match pattern %q", version, autoVersion.VersionPattern)
		}
		version = groups[1]
	}

	return version, nil
}

// extractJSONVariablesFromExtractBlock extracts all variables from the extract block
func extractJSONVariablesFromExtractBlock(client *http.Client, jsonConfig *manifest.JSONAutoVersionBlock, _ string) (map[string]map[string]string, error) {
	if jsonConfig.Extract == nil {
		return make(map[string]map[string]string), nil
	}

	// Fetch JSON data
	req, err := http.NewRequestWithContext(context.Background(), "GET", jsonConfig.URL, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "could not create request for variable extraction")
	}

	// Add custom headers
	for key, value := range jsonConfig.Headers {
		req.Header.Set(key, value)
	}

	// Set default Accept header if not specified
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "could not retrieve JSON for variable extraction")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "could not read response body")
	}

	// Parse JSON and extract variables for each platform
	if !gjson.ValidBytes(body) {
		return nil, errors.Errorf("invalid JSON response")
	}

	result := make(map[string]map[string]string)

	// Extract Darwin variables
	if jsonConfig.Extract.Darwin != nil {
		platformVars := make(map[string]string)
		if jsonConfig.Extract.Darwin.BuildNumber != "" {
			value := gjson.GetBytes(body, jsonConfig.Extract.Darwin.BuildNumber)
			if value.Exists() {
				platformVars["build_number"] = value.String()
			}
		}
		if jsonConfig.Extract.Darwin.SHA256 != "" {
			value := gjson.GetBytes(body, jsonConfig.Extract.Darwin.SHA256)
			if value.Exists() {
				platformVars["sha256"] = value.String()
			}
		}
		if jsonConfig.Extract.Darwin.CommitSHA != "" {
			value := gjson.GetBytes(body, jsonConfig.Extract.Darwin.CommitSHA)
			if value.Exists() {
				platformVars["commit_sha"] = value.String()
			}
		}
		if len(platformVars) > 0 {
			result["darwin"] = platformVars
		}
	}

	// Extract Linux variables
	if jsonConfig.Extract.Linux != nil {
		platformVars := make(map[string]string)
		if jsonConfig.Extract.Linux.BuildNumber != "" {
			value := gjson.GetBytes(body, jsonConfig.Extract.Linux.BuildNumber)
			if value.Exists() {
				platformVars["build_number"] = value.String()
			}
		}
		if jsonConfig.Extract.Linux.SHA256 != "" {
			value := gjson.GetBytes(body, jsonConfig.Extract.Linux.SHA256)
			if value.Exists() {
				platformVars["sha256"] = value.String()
			}
		}
		if jsonConfig.Extract.Linux.CommitSHA != "" {
			value := gjson.GetBytes(body, jsonConfig.Extract.Linux.CommitSHA)
			if value.Exists() {
				platformVars["commit_sha"] = value.String()
			}
		}
		if len(platformVars) > 0 {
			result["linux"] = platformVars
		}
	}

	// Extract Platform variables
	for _, platformBlock := range jsonConfig.Extract.Platform {
		platformVars := make(map[string]string)
		if platformBlock.BuildNumber != "" {
			value := gjson.GetBytes(body, platformBlock.BuildNumber)
			if value.Exists() {
				platformVars["build_number"] = value.String()
			}
		}
		if platformBlock.SHA256 != "" {
			value := gjson.GetBytes(body, platformBlock.SHA256)
			if value.Exists() {
				platformVars["sha256"] = value.String()
			}
		}
		if platformBlock.CommitSHA != "" {
			value := gjson.GetBytes(body, platformBlock.CommitSHA)
			if value.Exists() {
				platformVars["commit_sha"] = value.String()
			}
		}
		if len(platformVars) > 0 {
			// For platform blocks, we need to determine the platform name
			// For now, we'll use "platform" as the key - this might need refinement
			result["platform"] = platformVars
		}
	}

	return result, nil
}

// writeAutoVersionVarsCache writes a vars block inside the auto-version block
func writeAutoVersionVarsCache(versionBlock *hcl.Block, version string, versionData map[string]map[string]string) {
	if len(versionData) == 0 {
		return
	}

	// Find the auto-version block within the version block
	var autoVersionBlock *hcl.Block
	for _, entry := range versionBlock.Body {
		if entry.Block != nil && entry.Block.Name == "auto-version" {
			autoVersionBlock = entry.Block
			break
		}
	}

	if autoVersionBlock == nil {
		return // No auto-version block found
	}

	// Find or create vars block within auto-version
	var varsBlock *hcl.Block
	for _, entry := range autoVersionBlock.Body {
		if entry.Block != nil && entry.Block.Name == "vars" {
			varsBlock = entry.Block
			break
		}
	}

	if varsBlock == nil {
		// Create new vars block
		varsBlock = &hcl.Block{
			Name: "vars",
			Body: []*hcl.Entry{},
		}
		autoVersionBlock.Body = append(autoVersionBlock.Body, &hcl.Entry{
			Block: varsBlock,
		})
	}

	// Create version block within vars
	newVersionBlock := &hcl.Block{
		Name:   version,
		Labels: []string{},
		Body:   []*hcl.Entry{},
	}

	// Add platform blocks or top-level vars
	for platform, vars := range versionData {
		if len(vars) == 0 {
			continue
		}

		if platform == "" {
			// Top-level variables (no platform block)
			for key, value := range vars {
				newVersionBlock.Body = append(newVersionBlock.Body, &hcl.Entry{
					Attribute: &hcl.Attribute{
						Key:   key,
						Value: &hcl.Value{Str: &value},
					},
				})
			}
		} else {
			// Platform-specific variables
			platformBlock := &hcl.Block{
				Name: platform,
				Body: []*hcl.Entry{},
			}

			for key, value := range vars {
				platformBlock.Body = append(platformBlock.Body, &hcl.Entry{
					Attribute: &hcl.Attribute{
						Key:   key,
						Value: &hcl.Value{Str: &value},
					},
				})
			}

			newVersionBlock.Body = append(newVersionBlock.Body, &hcl.Entry{
				Block: platformBlock,
			})
		}
	}

	// Add version block to vars
	varsBlock.Body = append(varsBlock.Body, &hcl.Entry{
		Block: newVersionBlock,
	})
}
