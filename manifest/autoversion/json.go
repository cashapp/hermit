package autoversion

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/tidwall/gjson"
)

// JSONExtractionResult holds all extracted values from a JSON response.
type JSONExtractionResult struct {
	Version   string
	Variables map[string]string
	SHA256    string
}

// Auto-version by extracting version information from a JSON URL using JSONPath.
func jsonAutoVersion(client *http.Client, autoVersion *manifest.AutoVersionBlock) (version string, err error) {
	result, err := extractFromJSON(client, autoVersion)
	if err != nil {
		return "", err
	}
	return result.Version, nil
}

// extractFromJSON performs comprehensive JSON extraction including version, variables, and checksums.
func extractFromJSON(client *http.Client, autoVersion *manifest.AutoVersionBlock) (*JSONExtractionResult, error) {
	versionRe, err := regexp.Compile(autoVersion.VersionPattern)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	url := autoVersion.JSON.URL
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "could not create request for auto-version information")
	}

	// Add custom headers if specified
	for key, value := range autoVersion.JSON.Headers {
		req.Header.Set(key, value)
	}

	// Set default Accept header if not specified
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "could not retrieve auto-version information")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}

	// Read the entire response body for gjson parsing
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "%s: could not read response body", url)
	}

	// Validate that it's valid JSON
	if !gjson.ValidBytes(body) {
		return nil, errors.Errorf("%s: invalid JSON response", url)
	}

	// Extract version
	candidates, err := extractVersions(body, autoVersion.JSON.Path)
	if err != nil {
		return nil, errors.Wrapf(err, "%s: could not extract versions", url)
	}

	// Parse and sort versions so we can get the latest.
	versions := make(manifest.Versions, 0, len(candidates))
	for _, value := range candidates {
		value = strings.TrimSpace(value)
		groups := versionRe.FindStringSubmatch(value)
		if groups == nil {
			if autoVersion.IgnoreInvalidVersions {
				continue
			}
			return nil, errors.Errorf("version must match the pattern %s but is %s", autoVersion.VersionPattern, value)
		}
		versions = append(versions, manifest.ParseVersion(groups[1]))
	}
	sort.Sort(versions)

	if len(versions) == 0 {
		return nil, errors.Errorf("no versions matched on %s", url)
	}

	result := &JSONExtractionResult{
		Version:   versions[len(versions)-1].String(),
		Variables: make(map[string]string),
	}

	// Extract additional variables if specified
	for varName, varPath := range autoVersion.JSON.Vars {
		values, err := extractValues(body, varPath)
		if err != nil {
			return nil, errors.Wrapf(err, "%s: could not extract variable %s", url, varName)
		}
		if len(values) > 0 {
			result.Variables[varName] = values[0] // Use first match
		}
	}

	// Extract SHA256 checksum if specified
	if autoVersion.JSON.SHA256Path != "" {
		checksums, err := extractValues(body, autoVersion.JSON.SHA256Path)
		if err != nil {
			return nil, errors.Wrapf(err, "%s: could not extract SHA256", url)
		}
		if len(checksums) > 0 {
			result.SHA256 = checksums[0]
		}
	}

	return result, nil
}

// extractVersions extracts version strings from JSON data using gjson JSONPath.
func extractVersions(data []byte, path string) ([]string, error) {
	result := gjson.GetBytes(data, path)
	if !result.Exists() {
		return nil, errors.Errorf("JSONPath query %s matched no results", path)
	}

	var candidates []string
	if result.IsArray() {
		result.ForEach(func(key, value gjson.Result) bool {
			if value.Type == gjson.String {
				candidates = append(candidates, value.String())
			} else {
				candidates = append(candidates, value.Raw)
			}
			return true
		})
	} else if result.Type == gjson.String {
		candidates = append(candidates, result.String())
	} else {
		// For non-string values, use the raw JSON
		candidates = append(candidates, result.Raw)
	}

	return candidates, nil
}

// extractValues extracts arbitrary values from JSON data using gjson JSONPath.
func extractValues(data []byte, path string) ([]string, error) {
	result := gjson.GetBytes(data, path)
	if !result.Exists() {
		return nil, errors.Errorf("JSONPath query %s matched no results", path)
	}

	var candidates []string
	if result.IsArray() {
		result.ForEach(func(key, value gjson.Result) bool {
			if value.Type == gjson.String {
				candidates = append(candidates, value.String())
			} else {
				candidates = append(candidates, value.Raw)
			}
			return true
		})
	} else if result.Type == gjson.String {
		candidates = append(candidates, result.String())
	} else {
		// For non-string values, use the raw JSON
		candidates = append(candidates, result.Raw)
	}

	return candidates, nil
}
