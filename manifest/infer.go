package manifest

import (
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/platform"
	"github.com/cashapp/hermit/ui"
)

var (
	inferVerRe         = regexp.MustCompile(`releases/download/(?:v)?([^/]+)/`)
	inferOSRe          = regexp.MustCompile(`(darwin|linux)`)
	inferArchRe        = regexp.MustCompile(`(amd64|arm64)`)
	inferXArchRe       = regexp.MustCompile(`(x86_64|aarch64)`)
	darwinamd64        = platform.Platform{OS: platform.Darwin, Arch: platform.Amd64}
	darwinarm64        = platform.Platform{OS: platform.Darwin, Arch: platform.Arm64}
	suffixAlternatives = []string{".zip", ".tar.gz", ".tar.bz2", ".tgz", ".tbz2", ".tar.xz", ".txz"}
)

// InferFromArtefact attempts to infer a Manifest from a package artefact.
//
// "url" should be the full URL to a package artefact.
//
//     https://github.com/protocolbuffers/protobuf-go/releases/download/v1.27.1/protoc-gen-go.v1.27.1.darwin.amd64.tar.gz
//
// "version" may be specified if it cannot be inferred from the URL.
func InferFromArtefact(p *ui.UI, httpClient *http.Client, ghClient *github.Client, url, version string) (*Manifest, error) {
	source, version, err := insertVariables(url, version, false)
	if err != nil {
		return nil, err
	}
	// Pull description from GH API if possible.
	description := ""
	repoName := ghClient.ProjectForURL(url)
	if repoName != "" {
		repo, err := ghClient.Repo(repoName)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		description = repo.Description
	}

	// Check if sources for all valid platforms are available.
	sourcesByPlatform := buildSourcesByPlatform(version, source)

	p.Debugf("Validating sources")
	substituteM1, err := validateSourcesByPlatform(p, httpClient, sourcesByPlatform)
	if err != nil {
		return nil, err
	}

	// Substitute back in variables now that we have concrete URLs for each platform.
	for plat, platSource := range sourcesByPlatform {
		skip := substituteM1 && plat == darwinarm64
		platSource, _, err = insertVariables(platSource, version, skip)
		if err != nil {
			return nil, err
		}
		sourcesByPlatform[plat] = platSource
	}

	platforms := make([]*PlatformBlock, 0, len(sourcesByPlatform))
	for plat, platSource := range sourcesByPlatform {
		platforms = append(platforms, &PlatformBlock{
			Attrs: []string{plat.OS, plat.Arch},
			Layer: Layer{
				Source: platSource,
			},
		})
	}

	sort.Slice(platforms, func(i, j int) bool {
		return strings.Join(platforms[i].Attrs, "/") < strings.Join(platforms[j].Attrs, "/")
	})

	return &Manifest{
		Description: description,
		Layer: Layer{
			Binaries: []string{},
			Platform: platforms,
		},
		Versions: []VersionBlock{{
			Version: []string{version},
			AutoVersion: &AutoVersionBlock{
				GitHubRelease:  repoName,
				VersionPattern: "v?(.*)", // This is the default, which prevents the attribute being serialised.
			},
		}},
	}, nil
}

func buildSourcesByPlatform(version string, source string) map[platform.Platform]string {
	sourcesByPlatform := map[platform.Platform]string{}
	for _, plat := range platform.Core {
		vars := map[string]string{
			"version": version,
			"os":      plat.OS,
			"arch":    plat.Arch,
			"xarch":   platform.ArchToXArch(plat.Arch),
		}
		sourcesByPlatform[plat] = os.Expand(source, func(key string) string {
			return vars[key]
		})
	}
	return sourcesByPlatform
}

// Check that our initial sources exist and if not, try the same URLs with different extensions (.zip, .tgz, etc.)
func validateSourcesByPlatform(p *ui.UI, httpClient *http.Client, sourcesByPlatform map[platform.Platform]string) (substituteM1 bool, err error) {
nextPlatform:
	for plat, url := range sourcesByPlatform {
		p.Debugf("  %s - %s", plat, url)
		if err := ValidatePackageSource(httpClient, url); err != nil {
			// Try different extensions as packages sometimes use .zip for Windows, .tar.gz for Linux, etc.
			candidate := url
			// Strip the existing suffix.
			for _, suffix := range suffixAlternatives {
				candidate = strings.TrimSuffix(candidate, suffix)
			}
			// Try alternative suffixAlternatives.
			for _, suffix := range suffixAlternatives {
				url = candidate + suffix
				if err := ValidatePackageSource(httpClient, url); err == nil {
					sourcesByPlatform[plat] = url
					continue nextPlatform
				}
			}

			// Next just try to rely on Rosetta.
			if plat == darwinarm64 {
				substituteM1 = true
				delete(sourcesByPlatform, darwinarm64)
			} else {
				return false, errors.WithStack(err)
			}
		}
	}

	// We can substitute amd64 for arm64 on Darwin due to Rosetta.
	if substituteM1 {
		sourcesByPlatform[darwinarm64] = sourcesByPlatform[darwinamd64]
	}
	return substituteM1, nil
}

func insertVariables(url, inVersion string, skipArch bool) (source, version string, err error) {
	version = inVersion
	// Pull out variables.
	if version == "" {
		groups := inferVerRe.FindStringSubmatch(url)
		if len(groups) == 2 {
			version = groups[1]
		} else {
			return "", "", errors.Errorf("%s: could not infer version", url)
		}
	}
	pkgOS := inferOSRe.FindString(url)
	if pkgOS == "" {
		return "", "", errors.Errorf("%s: could not infer OS", url)
	}
	var arch, xarch string
	if !skipArch {
		arch = inferArchRe.FindString(url)
		xarch = inferXArchRe.FindString(url)
		if xarch == "" && arch == "" {
			return "", "", errors.Errorf("%s: could not infer CPU architecture", url)
		}
	}

	// Substitute in variables.
	source = strings.ReplaceAll(url, version, "${version}")
	source = strings.ReplaceAll(source, pkgOS, "${os}")
	if arch != "" {
		source = strings.ReplaceAll(source, arch, "${arch}")
	} else if xarch != "" {
		source = strings.ReplaceAll(source, xarch, "${xarch}")
	}
	return source, version, nil
}
