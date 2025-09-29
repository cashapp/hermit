package github

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cashapp/hermit/errors"
)

// MockRelease represents a GitHub release to be served by the mock server
type MockRelease struct {
	Repo    string   // owner/repo format
	TagName string   // tag name for the release
	Name    string   // name of the release asset
	Files   []string // paths to files to include in the release
}

// toInternalRelease converts a MockRelease to the internal representation
func (mr *MockRelease) toInternalRelease() (*Release, map[string][]byte, error) {
	files := make(map[string][]byte)
	for _, file := range mr.Files {
		// Create a tar.gz archive containing the file
		archiveContent, err := createArchiveFromFile(file)
		if err != nil {
			return nil, nil, errors.Wrap(err, "creating archive")
		}
		files[mr.Name] = archiveContent
	}

	release := &Release{
		TagName: mr.TagName,
		Assets: []Asset{
			{
				Name: mr.Name,
			},
		},
	}

	return release, files, nil
}

type MockGitHubServer struct {
	*httptest.Server
	releases map[string]map[string]*Release // map[repo][tag]release
	assets   map[string][]byte              // map[assetName]content
	config   mockServerConfig
}

type mockServerConfig struct {
	requiredAuthorizationToken string
	releases                   []MockRelease
}

// MockServerOption is a function that configures the mock server
type MockServerOption func(*mockServerConfig)

// WithRequiredToken configures the mock server to require a specific authorization token
func WithRequiredToken(token string) MockServerOption {
	return func(c *mockServerConfig) {
		c.requiredAuthorizationToken = token
	}
}

// WithMockRelease adds a mock release to be served by the server
func WithMockRelease(release MockRelease) MockServerOption {
	return func(c *mockServerConfig) {
		c.releases = append(c.releases, release)
	}
}

func NewMockGitHubServer(t *testing.T, opts ...MockServerOption) *MockGitHubServer {
	t.Helper()
	m := &MockGitHubServer{
		releases: make(map[string]map[string]*Release),
		assets:   make(map[string][]byte),
	}

	// Apply options
	for _, opt := range opts {
		opt(&m.config)
	}

	m.Server = httptest.NewServer(http.HandlerFunc(m.ServeHTTP))

	// Add all provided releases
	for _, mr := range m.config.releases {
		if _, ok := m.releases[mr.Repo]; !ok {
			m.releases[mr.Repo] = make(map[string]*Release)
		}

		release, files, err := mr.toInternalRelease()
		if err != nil {
			panic(fmt.Sprintf("failed to create release: %v", err))
		}

		m.releases[mr.Repo][mr.TagName] = release

		// Store all files as assets
		for name, content := range files {
			m.assets[name] = content
		}
	}

	return m
}

// createArchiveFromFile creates a tar.gz archive containing a single file from the test fixture
func createArchiveFromFile(scriptPath string) ([]byte, error) {
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, errors.Wrap(err, "reading script file")
	}

	// Create a tar.gz archive containing the script
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add the script to the archive with the base filename
	hdr := &tar.Header{
		Name: filepath.Base(scriptPath),
		Mode: 0755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, errors.Wrap(err, "writing tar header")
	}
	if _, err := tw.Write(content); err != nil {
		return nil, errors.Wrap(err, "writing file content")
	}

	// Close the archive
	if err := tw.Close(); err != nil {
		return nil, errors.Wrap(err, "closing tar writer")
	}
	if err := gw.Close(); err != nil {
		return nil, errors.Wrap(err, "closing gzip writer")
	}

	return buf.Bytes(), nil
}

func (m *MockGitHubServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Verify authorization if required
	if m.config.requiredAuthorizationToken != "" {
		authHeader := r.Header.Get("Authorization")
		expectedHeader := "token " + m.config.requiredAuthorizationToken
		if authHeader != expectedHeader {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, "Unauthorized: expected token %q, got %q",
				m.config.requiredAuthorizationToken, strings.TrimPrefix(authHeader, "token "))
			return
		}
	}

	// Handle GitHub releases/tags API endpoint
	if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/repos/") {
		if strings.Contains(r.URL.Path, "/releases/tags/") {
			m.handleReleaseTags(w, r)
			return
		} else if strings.Contains(r.URL.Path, "/releases/download/") {
			m.handleReleaseDownload(w, r)
			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, "Not implemented")
}

func (m *MockGitHubServer) handleReleaseTags(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 6 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Invalid path format")
		return
	}

	// Extract repo and tag from path
	// Format: /repos/{owner}/{repo}/releases/tags/{tag}
	repo := fmt.Sprintf("%s/%s", parts[2], parts[3])
	tag := parts[len(parts)-1]

	repoReleases, ok := m.releases[repo]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Repository not found")
		return
	}

	release, ok := repoReleases[tag]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Release not found")
		return
	}

	// Ensure asset URLs point to our mock server
	for i := range release.Assets {
		release.Assets[i].URL = fmt.Sprintf("%s/repos/%s/releases/download/%s/%s",
			m.URL(), repo, release.TagName, release.Assets[i].Name)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(release); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (m *MockGitHubServer) handleReleaseDownload(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 7 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Invalid download path format")
		return
	}

	// Extract asset name from path
	// Format: /repos/{owner}/{repo}/releases/download/{tag}/{asset}
	assetName := parts[len(parts)-1]

	content, ok := m.assets[assetName]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Asset not found")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(content); err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
		return
	}
}

// URL returns the base URL of the mock server
func (m *MockGitHubServer) URL() string {
	return m.Server.URL
}
