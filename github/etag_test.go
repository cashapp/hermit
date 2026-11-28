package github_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/cashapp/hermit/github"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestETagReportsUnsuccessfulStatus(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusServiceUnavailable} {
		client := github.New(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: status,
				Status:     http.StatusText(status),
				Header:     http.Header{"Etag": []string{`"error-page"`}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		})}, "token")

		etag, err := client.ETag(github.Asset{Name: "asset", URL: "https://api.github.com/repos/o/r/releases/assets/1"})
		assert.Error(t, err)
		assert.Equal(t, "", etag)
	}
}
