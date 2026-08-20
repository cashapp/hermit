package github

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
)

func TestGHTokenFailureIsCached(t *testing.T) {
	previous := ghAuthToken
	defer func() { ghAuthToken = previous }()
	calls := 0
	ghAuthToken = func(string) (string, error) {
		calls++
		return "", errors.New("not authenticated")
	}

	transport := TokenAuthenticatedTransportForHosts(roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("request should not be sent without a token")
		return nil, nil
	}), []HostConfig{{WebHost: "mycompany.ghe.com"}})
	for range 2 {
		req, err := http.NewRequest(http.MethodGet, "https://api.mycompany.ghe.com/repos/owner/repo", nil)
		assert.NoError(t, err)
		_, err = transport.RoundTrip(req)
		assert.Error(t, err)
	}
	assert.Equal(t, 1, calls)
}

func TestGHTokenLookupsForDifferentHostsDoNotBlockEachOther(t *testing.T) {
	previous := ghAuthToken
	defer func() { ghAuthToken = previous }()
	started := make(chan string, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	closeRelease := func() { releaseOnce.Do(func() { close(release) }) }
	defer closeRelease()
	ghAuthToken = func(host string) (string, error) {
		started <- host
		<-release
		return "token", nil
	}

	transport := TokenAuthenticatedTransportForHosts(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	}), []HostConfig{{WebHost: "one.ghe.com"}, {WebHost: "two.ghe.com"}})

	var wg sync.WaitGroup
	for _, host := range []string{"one.ghe.com", "two.ghe.com"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodGet, "https://api."+host+"/repos/owner/repo", nil)
			assert.NoError(t, err)
			_, err = transport.RoundTrip(req)
			assert.NoError(t, err)
		}()
	}

	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("gh token lookups were serialized")
		}
	}
	closeRelease()
	wg.Wait()
}
