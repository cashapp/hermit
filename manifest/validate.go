package manifest

import (
	"github.com/cashapp/hermit/cache"
	"net/http"

	"github.com/pkg/errors"
)

// ValidatePackageSource checks that a package source is accessible.
func ValidatePackageSource(httpClient *http.Client, url string) error {
	source, err := cache.GetSource(url)
	if err != nil {
		return errors.Wrap(err, url)
	}
	return errors.Wrapf(source.Validate(httpClient), "invalid source")
}
