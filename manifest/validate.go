package manifest

import (
	"net/http"

	"github.com/cashapp/hermit/cache"

	"github.com/pkg/errors"
)

// ValidatePackageSource checks that a package source is accessible.
func ValidatePackageSource(packageSource cache.PackageSourceSelector, httpClient *http.Client, url string) error {
	source, err := packageSource(httpClient, url)
	if err != nil {
		return errors.Wrap(err, url)
	}
	return errors.Wrapf(source.Validate(), "invalid source")
}
