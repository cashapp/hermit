package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestNewHTTPClientNegotiatesHTTP2(t *testing.T) {
	tests := []struct {
		name        string
		serverHTTP2 bool
		want        string
	}{
		{"server with h2", true, "HTTP/2.0"},
		{"server without h2 falls back", false, "HTTP/1.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, r.Proto)
			}))
			server.EnableHTTP2 = tt.serverHTTP2
			server.StartTLS()
			t.Cleanup(server.Close)

			client := newHTTPClient(HTTPTransportConfig{})
			// Trust the test server's certificate.
			client.Transport.(*http.Transport).TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
			resp, err := client.Get(server.URL)
			assert.NoError(t, err)
			body, err := io.ReadAll(resp.Body)
			assert.NoError(t, err)
			assert.NoError(t, resp.Body.Close())
			assert.Equal(t, tt.want, string(body))
		})
	}
}
