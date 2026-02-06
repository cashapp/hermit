package cache

import (
	"net/http"
	"net/url"
)

// CachewSourceSelector wraps another PackageSourceSelector to redirect HTTP/HTTPS
// downloads through a Cachew proxy server https://github.com/block/cachew.
func CachewSourceSelector(getSource PackageSourceSelector, cachewURL string) PackageSourceSelector {
	return func(client *http.Client, uri string) (PackageSource, error) {
		u, err := url.Parse(uri)
		if err != nil {
			return getSource(client, uri)
		}

		// Only intercept HTTP and HTTPS schemes
		if u.Scheme != "http" && u.Scheme != "https" {
			return getSource(client, uri)
		}

		// Rewrite URL to go through Cachew proxy
		rewrittenURI := cachewURL + "/hermit/" + u.Host + u.Path
		if u.RawQuery != "" {
			rewrittenURI += "?" + u.RawQuery
		}

		return HTTPSource(client, rewrittenURI), nil
	}
}
