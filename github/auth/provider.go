package auth

import (
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
)

// Provider is an interface for GitHub token providers
type Provider interface {
	// GetToken returns a GitHub token or an error if one cannot be obtained
	GetToken() (string, error)
}

// EnvProvider implements Provider using environment variables
type EnvProvider struct {
	ui *ui.UI
}

// GetToken returns a token from environment variables
func (p *EnvProvider) GetToken() (string, error) {
	p.ui.Debugf("Getting GitHub token from environment variables")
	if token := os.Getenv("HERMIT_GITHUB_TOKEN"); token != "" {
		p.ui.Tracef("Using HERMIT_GITHUB_TOKEN for GitHub authentication")
		return token, nil
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		p.ui.Tracef("Using GITHUB_TOKEN for GitHub authentication")
		return token, nil
	}
	p.ui.Tracef("No GitHub token found in environment")
	return "", errors.New("no GitHub token found in environment")
}

// GHCliProvider implements Provider using the gh CLI tool
type GHCliProvider struct {
	// cache the token and only refresh when needed
	token     string
	tokenLock sync.Mutex
	ui        *ui.UI
}

// GetToken returns a token from gh CLI
func (p *GHCliProvider) GetToken() (string, error) {
	p.ui.Debugf("Getting GitHub token from gh CLI")
	p.tokenLock.Lock()
	defer p.tokenLock.Unlock()

	// Return cached token if available
	if p.token != "" {
		return p.token, nil
	}

	// Check if gh is installed
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		return "", errors.New("gh CLI not found in PATH")
	}

	p.ui.Tracef("gh found: %s", ghPath)

	// Run gh auth token
	cmd := exec.Command("gh", "auth", "token")
	output, err := cmd.Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to get token from gh CLI")
	}

	p.token = strings.TrimSpace(string(output))
	return p.token, nil
}

// NewProvider creates a new token provider based on the specified type
func NewProvider(providerType string, ui *ui.UI) (Provider, error) {
	switch providerType {
	case "env", "":
		return &EnvProvider{ui: ui}, nil
	case "gh-cli":
		return &GHCliProvider{ui: ui}, nil
	default:
		return nil, errors.Errorf("unknown GitHub token provider: %s", providerType)
	}
}
