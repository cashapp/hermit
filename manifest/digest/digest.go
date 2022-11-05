package digest

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

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
	mani, err := manifest.LoadManifestFile(os.DirFS(filepath.Dir(path)), filename)
	if err != nil {
		return errors.Wrap(err, "failed to load manifest")
	}
	// Dedupe by source, as channels often have the same source as normal packages.
	pkgsBySource := map[string]pkgAndref{}
	for _, ref := range mani.References(name) {
		for _, platform := range platform.Core {
			config := manifest.Config{Env: ".", State: "/tmp", Platform: platform}
			pkg, err := manifest.Resolve(mani, config, ref)
			if err != nil {
				return errors.WithStack(err)
			}
			// Skip checksums for channels.
			if pkg.Reference.Channel != "" {
				continue
			}
			existing, ok := pkgsBySource[pkg.Source]
			if ok && existing.pkg.SHA256 != "" {
				continue
			}
			pkgsBySource[pkg.Source] = pkgAndref{pkg, ref, platform}
		}
	}

	missing := 0
	for _, pkg := range pkgsBySource {
		if pkg.pkg.SHA256 == "" {
			missing++
		}
	}

	if missing == 0 {
		l.Infof("All packages have checksums!")
		return nil
	}

	l.Infof("Updating %d checksums...", missing)

	updated := []pkgAndDigest{}

	// Compute missing checksums
	for _, pkg := range pkgsBySource {
		if pkg.pkg.SHA256 != "" {
			l.Debugf("  %s %s (existing)", pkg.pkg.SHA256, pkg.pkg.Source)
			continue
		}
		digest, err := computeDigest(l, client, state, pkg.pkg, pkg.ref)
		if err != nil {
			return errors.Wrapf(err, "failed to compute digest for %s/%s", pkg.ref.String(), pkg.platform)
		}
		l.Infof("  %s %s", digest, pkg.pkg.Source)
		updated = append(updated, pkgAndDigest{pkg.pkg.Reference, pkg.pkg.Source, digest})
	}

	if len(updated) == 0 {
		return nil
	}

	// Update the HCL file with the new checksums
	ast, err := loadAST(path)
	if err != nil {
		return errors.WithStack(err)
	}

	err = updateHCLSHA256Sums(ast, updated)
	if err != nil {
		return errors.Wrap(err, filename)
	}

	err = writeAST(path, ast, filename)
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

func computeDigest(l *ui.UI, client *http.Client, state *state.State, pkg *manifest.Package, ref manifest.Reference) (string, error) {
	task := l.Task(ref.String())
	defer task.Done()

	// As an optimisation we'll first try <source>.sha256.txt
	if digest := tryGetSHA(client, pkg.Source+".sha256.txt"); digest != "" {
		l.Debugf("  %s.sha256.txt => %s", pkg.Source, digest)
		return digest, nil
	}

	digest, err := state.CacheAndDigest(task, pkg)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return digest, nil
}

var digestRe = regexp.MustCompile(`^([A-Z0-9a-z]{64}).*`)

func tryGetSHA(client *http.Client, url string) string {
	req, err := http.NewRequest(http.MethodGet, url, &strings.Reader{}) //nolint: noctx
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	if groups := digestRe.FindStringSubmatch(string(data)); groups != nil {
		return groups[1]
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
		pkg := pkg
		sha256Sums.Map = append(sha256Sums.Map, &hcl.MapEntry{
			Key:   &hcl.Value{Str: &pkg.source},
			Value: &hcl.Value{Str: &pkg.digest},
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
