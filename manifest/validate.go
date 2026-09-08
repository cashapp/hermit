package manifest

import (
	"net/http"

	"github.com/cashapp/hermit/cache"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/redact"
)

// ValidatePackageSource checks that a package source is accessible.
func ValidatePackageSource(packageSource cache.PackageSourceSelector, httpClient *http.Client, url redact.URL) error {
	source, err := packageSource(httpClient, url)
	if err != nil {
		return errors.Wrap(err, url.String())
	}
	return errors.Wrapf(source.Validate(), "invalid source")
}
