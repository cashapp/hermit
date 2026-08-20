package github

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestAPIBaseURL(t *testing.T) {
	assert.Equal(t, "https://api.github.com", APIBaseURL("github.com"))
	assert.Equal(t, "https://api.mycompany.ghe.com", APIBaseURL("https://mycompany.ghe.com/"))
	assert.Equal(t, "https://api.github.example.com", APIBaseURL("github.example.com"))
}

func TestNormalizeHost(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  string
	}{
		{"", "github.com"},
		{" GitHub.com ", "github.com"},
		{"https://MyCompany.ghe.com/", "mycompany.ghe.com"},
		{"mycompany.ghe.com:443", "mycompany.ghe.com"},
		{"mycompany.ghe.com:8443", "mycompany.ghe.com:8443"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeHost(tt.input))
		})
	}
}

func TestNewWithHostsDoesNotMutateHostConfigs(t *testing.T) {
	hosts := []HostConfig{{WebHost: "https://GitHub.com/"}, {WebHost: "MyCompany.ghe.com"}}
	want := append([]HostConfig(nil), hosts...)

	NewWithHosts(nil, hosts)

	assert.Equal(t, want, hosts)
}

func TestEnterpriseHostAPIAndConfiguredToken(t *testing.T) {
	var gotHost, gotPath, gotAuth string
	client := NewWithHosts(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotHost = r.URL.Host
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, err := json.Marshal(&Repo{Description: "private repo", Homepage: "https://example.com"})
		assert.NoError(t, err)
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
	})}, []HostConfig{{WebHost: "mycompany.ghe.com", Token: "enterprise-token"}})

	repo, err := client.Repo("mycompany.ghe.com", "owner/repo")
	assert.NoError(t, err)
	assert.Equal(t, "private repo", repo.Description)
	assert.Equal(t, "api.mycompany.ghe.com", gotHost)
	assert.Equal(t, "/repos/owner/repo", gotPath)
	assert.Equal(t, "token enterprise-token", gotAuth)
}

func TestEnterpriseHostTokenFromGHCLI(t *testing.T) {
	previous := ghAuthToken
	defer func() { ghAuthToken = previous }()
	var gotTokenHost string
	ghAuthToken = func(host string) (string, error) {
		gotTokenHost = host
		return "gh-token", nil
	}

	var gotAuth string
	client := NewWithHosts(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotAuth = r.Header.Get("Authorization")
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"description":"private repo"}`)), Request: r}, nil
	})}, []HostConfig{{WebHost: "mycompany.ghe.com"}})

	_, err := client.Repo("mycompany.ghe.com", "owner/repo")
	assert.NoError(t, err)
	assert.Equal(t, "mycompany.ghe.com", gotTokenHost)
	assert.Equal(t, "token gh-token", gotAuth)
}

func TestProjectForURLEnterpriseHost(t *testing.T) {
	client := NewWithHosts(nil, []HostConfig{{WebHost: "mycompany.ghe.com"}})
	host, project := client.ProjectForURL("https://mycompany.ghe.com/owner/repo/releases/download/v1/asset.tar.gz")
	assert.Equal(t, "mycompany.ghe.com", host)
	assert.Equal(t, "owner/repo", project)
	host, project = client.ProjectForURL("https://another.ghe.com/owner/repo/releases/download/v1/asset.tar.gz")
	assert.Equal(t, "another.ghe.com", host)
	assert.Equal(t, "owner/repo", project)
	host, project = client.ProjectForURL("https://github.example.com/owner/repo/releases/download/v1/asset.tar.gz")
	assert.Equal(t, "", host)
	assert.Equal(t, "", project)
	host, project = client.ProjectForURL("https://MYCOMPANY.GHE.COM:443/owner/repo/releases/download/v1/asset.tar.gz")
	assert.Equal(t, "mycompany.ghe.com", host)
	assert.Equal(t, "owner/repo", project)
}

func TestObjectStorageRedirectHostDoesNotUseGHToken(t *testing.T) {
	previous := ghAuthToken
	defer func() { ghAuthToken = previous }()
	ghAuthToken = func(host string) (string, error) {
		t.Fatalf("gh auth token should not be called for object storage host %s", host)
		return "", nil
	}

	var gotAuth string
	transport := TokenAuthenticatedTransportForHosts(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotAuth = r.Header.Get("Authorization")
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	}), []HostConfig{{WebHost: "mycompany.ghe.com"}})

	req, err := http.NewRequest(http.MethodGet, "https://objects-origin.mycompany.ghe.com/github-production-release-asset/file", nil)
	assert.NoError(t, err)
	_, err = transport.RoundTrip(req)
	assert.NoError(t, err)
	assert.Equal(t, "", gotAuth)
}

func TestEnterpriseAssetURLCannotUseGitHubDotCom(t *testing.T) {
	client := NewWithHosts(nil, []HostConfig{{WebHost: "github.com"}, {WebHost: "mycompany.ghe.com"}})

	_, err := client.Download("mycompany.ghe.com", Asset{URL: "https://api.github.com/repos/mycompany/tool/releases/assets/1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `does not match configured GitHub host "mycompany.ghe.com"`)
}

func TestEnterpriseAssetURLHostIsNormalized(t *testing.T) {
	client := NewWithHosts(nil, []HostConfig{{WebHost: "mycompany.ghe.com"}})

	err := client.validateAssetURLForHost("mycompany.ghe.com", "https://API.MYCOMPANY.GHE.COM:443/repos/mycompany/tool/releases/assets/1")
	assert.NoError(t, err)
}

func TestEnterpriseHostMissingGHTokenFails(t *testing.T) {
	previous := ghAuthToken
	defer func() { ghAuthToken = previous }()
	ghAuthToken = func(host string) (string, error) {
		return "", errors.New("gh missing")
	}

	client := NewWithHosts(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("request should not be sent without a token")
		return nil, nil
	})}, []HostConfig{{WebHost: "mycompany.ghe.com"}})

	_, err := client.Repo("mycompany.ghe.com", "owner/repo")
	assert.Error(t, err)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (r roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}
