package cache

import (
	"net/http"
	"testing"
)

func TestCachewSourceSelector(t *testing.T) {
	// Mock base selector that always returns a file source
	baseSelector := func(client *http.Client, uri string) (PackageSource, error) {
		return &fileSource{path: "/tmp/test"}, nil
	}

	cachewURL := "https://cachew.example.com:8080"
	selector := CachewSourceSelector(baseSelector, cachewURL)

	tests := []struct {
		name        string
		inputURI    string
		expectedURI string
		shouldProxy bool
	}{
		{
			name:        "HTTP URL should be proxied",
			inputURI:    "http://example.com/file.tar.gz",
			expectedURI: "https://cachew.example.com:8080/hermit/example.com/file.tar.gz",
			shouldProxy: true,
		},
		{
			name:        "HTTPS URL should be proxied",
			inputURI:    "https://github.com/owner/repo/releases/download/v1.0/file.tar.gz",
			expectedURI: "https://cachew.example.com:8080/hermit/github.com/owner/repo/releases/download/v1.0/file.tar.gz",
			shouldProxy: true,
		},
		{
			name:        "HTTPS URL with query params should preserve params",
			inputURI:    "https://example.com/file.tar.gz?version=1.0&os=linux",
			expectedURI: "https://cachew.example.com:8080/hermit/example.com/file.tar.gz?version=1.0&os=linux",
			shouldProxy: true,
		},
		{
			name:        "File URL should not be proxied",
			inputURI:    "file:///tmp/local-file.tar.gz",
			expectedURI: "",
			shouldProxy: false,
		},
		{
			name:        "Git URL should not be proxied",
			inputURI:    "git://github.com/owner/repo.git",
			expectedURI: "",
			shouldProxy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, err := selector(nil, tt.inputURI)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.shouldProxy {
				httpSrc, ok := source.(*httpSource)
				if !ok {
					t.Fatalf("expected httpSource, got %T", source)
				}
				if httpSrc.url != tt.expectedURI {
					t.Errorf("expected URL %s, got %s", tt.expectedURI, httpSrc.url)
				}
			} else {
				// Should fall through to base selector
				fileSrc, ok := source.(*fileSource)
				if !ok {
					t.Fatalf("expected fileSource (from base selector), got %T", source)
				}
				if fileSrc.path != "/tmp/test" {
					t.Errorf("expected path from base selector, got %s", fileSrc.path)
				}
			}
		})
	}
}

func TestCachewSourceSelectorInvalidURL(t *testing.T) {
	baseSelector := func(client *http.Client, uri string) (PackageSource, error) {
		return &fileSource{path: "/tmp/fallback"}, nil
	}

	selector := CachewSourceSelector(baseSelector, "https://cachew.example.com")

	// Test invalid URL - should fall back to base selector
	source, err := selector(nil, "://invalid-url")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fileSrc, ok := source.(*fileSource)
	if !ok {
		t.Fatalf("expected fileSource (fallback), got %T", source)
	}
	if fileSrc.path != "/tmp/fallback" {
		t.Errorf("expected fallback path, got %s", fileSrc.path)
	}
}
