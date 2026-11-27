package github

import (
	"net/http"
	"testing"
	"time"

	"github.com/cashapp/hermit/redact"
)

type sentinelTransport struct{}

func (sentinelTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, nil }

func TestNewKeepsConfiguredClientWithoutToken(t *testing.T) {
	configured := &http.Client{Transport: sentinelTransport{}, Timeout: 3 * time.Second}
	c := New(configured, "")
	if c.client.Transport != configured.Transport {
		t.Fatalf("expected configured transport to be kept, got %T", c.client.Transport)
	}
	if c.client.Timeout != configured.Timeout {
		t.Fatalf("expected configured timeout %v, got %v", configured.Timeout, c.client.Timeout)
	}
}

func TestNewKeepsClientSettingsWithToken(t *testing.T) {
	configured := &http.Client{Transport: sentinelTransport{}, Timeout: 3 * time.Second}
	c := New(configured, redact.Secret("tok"))
	if c.client.Timeout != configured.Timeout {
		t.Fatalf("expected configured timeout %v, got %v", configured.Timeout, c.client.Timeout)
	}
	if _, ok := c.client.Transport.(*githubAuthenticatedHTTPClient); !ok {
		t.Fatalf("expected authenticated transport, got %T", c.client.Transport)
	}
}
