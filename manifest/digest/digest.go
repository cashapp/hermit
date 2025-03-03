package digest

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/alecthomas/hcl"
	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/platform"
	"github.com/cashapp/hermit/state"
	"github.com/cashapp/hermit/ui"
)

// UpdateDigests for the manifest at the given path.
//
// Sources with existing digests will be skipped. All sources for the core
// supported platforms will be checked.
func UpdateDigests(l *ui.UI, client *http.Client, state *state.State, path string) error {
	filename := filepath.Base(path)
	name := strings.TrimSuffix(filename, ".hcl")
	task := l.Task(name)
	defer task.Done()
	mani, err := manifest.LoadManifestFile(os.DirFS(filepath.Dir(path)), filename)
	if err != nil {
		return errors.Wrap(err, "failed to load manifest")
	}
	// Dedupe by source, as channels often have the same source as normal packages.
	pkgsBySource := map[string]pkgAndref{}
	for _, ref := range mani.References(name) {
		for _, p := range slices.Concat(platform.Core, platform.Optional) {
			config := manifest.Config{Env: ".", State: "/tmp", Platform: p}
			pkg, err := manifest.Resolve(mani, config, ref)
			if errors.Is(err, manifest.ErrNoSource) {
				maybeWarnf(task, p, "No source provided for %s/%s", ref, p)
				continue
			}
			if err != nil {
				if slices.Contains(platform.Core, p) {
					return errors.WithStack(err)
				}
				task.Debugf("Cannot resolve optional digest update for %s/%s, skipping: %s", ref, p, err)
				continue
			}
			// Skip git repos
			if strings.Contains(pkg.Source, ".git#") || strings.HasSuffix(pkg.Source, ".git") {
				continue
			}
			// Skip checksums for channels.
			if pkg.Reference.Channel != "" {
				continue
			}
			existing, ok := pkgsBySource[pkg.Source]
			if ok && existing.pkg.SHA256 != "" {
				continue
			}
			pkgsBySource[pkg.Source] = pkgAndref{pkg, ref, p}
		}
	}

	missing := 0
	for _, pkg := range pkgsBySource {
		if pkg.pkg.SHA256 == "" {
			missing++
		}
	}

	if missing == 0 {
		task.Infof("All packages have checksums!")
		return nil
	}

	task.Infof("Updating %d checksums...", missing)

	updated := []pkgAndDigest{}

	// Compute missing checksums
	for _, pkg := range pkgsBySource {
		if pkg.pkg.SHA256 != "" {
			task.Debugf("  %s %s (existing)", pkg.pkg.SHA256, pkg.pkg.Source)
			continue
		}
		digest, err := computeDigest(task, client, state, pkg.pkg)
		if err != nil {
			if slices.Contains(platform.Core, pkg.platform) {
				return errors.Wrapf(err, "failed to compute digest for %s/%s", pkg.ref.String(), pkg.platform)
			}
			task.Debugf("Cannot compute optional digest update for %s/%s, skipping: %s", pkg.ref.String(), pkg.platform, err)
			continue
		}
		task.Infof("  %s %s", digest, pkg.pkg.Source)
		updated = append(updated, pkgAndDigest{pkg.pkg.Reference, pkg.pkg.Source, digest})
		if len(updated) > 10 {
			err := snapshotDigests(path, updated)
			if err != nil {
				return errors.WithStack(err)
			}
			updated = []pkgAndDigest{}
		}
	}

	if len(updated) == 0 {
		return nil
	}

	return snapshotDigests(path, updated)
}

func snapshotDigests(path string, updated []pkgAndDigest) error {
	// Update the HCL file with the new checksums
	ast, err := loadAST(path)
	if err != nil {
		return errors.WithStack(err)
	}

	err = updateHCLSHA256Sums(ast, updated)
	if err != nil {
		return errors.Wrap(err, path)
	}

	err = writeAST(path, ast, path)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func writeAST(path string, ast *hcl.AST, filename string) error {
	w, err := os.Create(path)
	if err != nil {
		return errors.WithStack(err)
	}
	defer w.Close()
	err = hcl.MarshalASTToWriter(ast, w)
	if err != nil {
		return errors.Wrap(err, filename)
	}
	return nil
}

type pkgAndDigest struct {
	ref    manifest.Reference
	source string
	digest string
}

type pkgAndref struct {
	pkg      *manifest.Package
	ref      manifest.Reference
	platform platform.Platform
}

func computeDigest(task *ui.Task, client *http.Client, state *state.State, pkg *manifest.Package) (string, error) {
	// As an optimisation we'll first try <source>.sha256.txt
	if digest := tryGetSHA(task, client, pkg); digest != "" {
		return digest, nil
	}

	digest, err := state.CacheAndDigest(task, pkg)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return digest, nil
}

var digestRe = regexp.MustCompile(`^([A-Z0-9a-z]{64})(?:\s+(.*))?`)

var checksumCache sync.Map

func tryGetSHA(task *ui.Task, client *http.Client, pkg *manifest.Package) string {
	u := pkg.Source
	dir := u[:strings.LastIndex(u, "/")]
	variants := []string{u + ".sha256.txt", u + ".sha256", dir + "/checksums.txt", dir + "/sha256.txt", dir + "/SHA256SUMS"}
	if pkg.SHA256Source != "" {
		variants = []string{pkg.SHA256Source}
	}
	filename, err := url.PathUnescape(path.Base(u))
	if err != nil {
		filename = u
	}
	for _, variant := range variants {
		content, ok := checksumCache.Load(variant)
		if !ok {
			task.Tracef("Trying %s", variant)
			req, err := http.NewRequest(http.MethodGet, variant, &strings.Reader{}) //nolint: noctx
			if err != nil {
				return ""
			}
			resp, err := client.Do(req)
			if err != nil {
				task.Tracef("Failed to fetch %s: %v", variant, err)
				return ""
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				_ = resp.Body.Close()
				task.Tracef("%s %s", variant, resp.Status)
				continue
			}
			data, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				task.Tracef("Failed to read %s: %v", variant, err)
				continue
			}
			content = string(data)
			checksumCache.Store(variant, content)
		}
		lines := strings.Split(strings.TrimSpace(content.(string)), "\n") // nolint
		allowMissingFilename := len(lines) == 1
		for _, line := range lines {
			task.Tracef("%s: %s", variant, line)
			groups := digestRe.FindStringSubmatch(line)
			if len(groups) > 0 && (allowMissingFilename || strings.EqualFold(groups[2], filename)) {
				task.Tracef("Short-circuit match %s", variant)
				return groups[1]
			}
		}
	}
	return ""
}

// UpdateChecksums for the manifest at the given path.
func updateHCLSHA256Sums(ast *hcl.AST, updated []pkgAndDigest) error {
	sort.Slice(updated, func(i, j int) bool {
		return updated[i].ref.Compare(updated[j].ref) < 0
	})

	sha256Sums, err := upsertSHA256SumsKey(ast)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, pkg := range updated {
		sha256Sums.Map = append(sha256Sums.Map, &hcl.MapEntry{
			Key:   &hcl.Value{Str: &pkg.source}, //nolint:exportloopref
			Value: &hcl.Value{Str: &pkg.digest}, //nolint:exportloopref
		})
	}

	return nil
}

func upsertSHA256SumsKey(ast *hcl.AST) (*hcl.Value, error) {
	var sha256Sums *hcl.Value

	for _, v := range ast.Entries {
		if v.Attribute != nil && v.Attribute.Key != "" {
			if v.Attribute.Key == "sha256sums" {
				if !v.Attribute.Value.HaveMap {
					return nil, errors.Errorf("%s: sha256sums is not a map", v.Attribute.Pos)
				}
				sha256Sums = v.Attribute.Value
				break
			}
		}
	}

	if sha256Sums == nil {
		sha256Sums = &hcl.Value{HaveMap: true}
		ast.Entries = append(ast.Entries, &hcl.Entry{Attribute: &hcl.Attribute{Key: "sha256sums", Value: sha256Sums}})
	}
	return sha256Sums, nil
}

func loadAST(path string) (*hcl.AST, error) {
	r, err := os.Open(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer r.Close()
	ast, err := hcl.Parse(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return ast, nil
}

// Only emit a warning for core platforms to keep output from being noisy
func maybeWarnf(task *ui.Task, p platform.Platform, format string, args ...interface{}) {
	if slices.Contains(platform.Core, p) {
		task.Warnf(format, args...)
	} else {
		task.Debugf(format, args...)
	}
}
