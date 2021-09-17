package manifest

import (
	"context"
	"net/http"

	"github.com/pkg/errors"
)

// ValidatePackageSource checks that a package source is accessible.
func ValidatePackageSource(httpClient *http.Client, url string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodHead, url, nil)
	if err != nil {
		return errors.Wrap(err, url)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return errors.WithStack(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return errors.Errorf("could not retrieve source archive from %s: %s", url, resp.Status)
	}

	return nil
}
