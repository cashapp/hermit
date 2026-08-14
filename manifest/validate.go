package manifest

import (
	"net/http"

	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/sources"
)

// ValidatePackageSource checks that a package source is accessible.
func ValidatePackageSource(packageSource cache.PackageSourceSelector, httpClient *http.Client, url string) error {
	urlSource := sources.NewSource(url)
	source, err := packageSource(httpClient, urlSource)
	if err != nil {
		return errors.Wrap(err, urlSource.String())
	}
	return errors.Wrapf(source.Validate(), "invalid source")
}
