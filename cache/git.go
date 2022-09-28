package cache

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

type gitSource struct {
	URL string
}

func (s *gitSource) OpenLocal(c *Cache, checksum string) (*os.File, error) {
	f, err := os.Open(c.Path(checksum, s.URL))
	return f, errors.WithStack(err)
}

func (s *gitSource) Download(b *ui.Task, cache *Cache, checksum string) (string, string, string, error) {
	base := BasePath(checksum, s.URL)
	checkoutDir := filepath.Join(cache.root, base)
	repo, tag := parseGitURL(s.URL)
	args := []string{"git", "clone", "--depth=1", repo, checkoutDir}
	if tag != "" {
		args = append(args, "--branch="+tag)
	}
	err := util.RunInDir(b, cache.root, args...)
	if err != nil {
		return "", "", "", errors.WithStack(err)
	}

	bts, err := util.CaptureInDir(b, checkoutDir, "git", "rev-parse", "HEAD")
	// For git sources there is no need for an actual checksum.
	// A tuple of git_source_url, commit is a unique enough combination.
	// Also, it's hard to make sense of what this checksum would be.
	// In other implementations of the Download interface function, it's sha256 value
	// but here there is no obvious definition.
	var sbts = strings.TrimSpace(string(bts))
	if checksum == "" {
		base = BasePath(sbts, s.URL)
	}
	if err != nil {
		return "", "", "", errors.WithStack(err)
	}

	return filepath.Join(cache.root, base), sbts, sbts, nil
}

func (s *gitSource) ETag(b *ui.Task) (etag string, err error) {
	repo, tag := parseGitURL(s.URL)
	if tag == "" {
		tag = "HEAD"
	}
	bts, err := util.Capture(b, "git", "ls-remote", repo, tag)
	if err != nil {
		return "", errors.Wrap(err, s.URL)
	}
	str := string(bts)
	parts := strings.Split(str, "\t")
	if len(parts) != 2 {
		return "", errors.Errorf("invalid HEAD: %s", str)
	}

	return parts[0], nil
}

func (s *gitSource) Validate() error {
	repo, tag := parseGitURL(s.URL)
	if tag == "" {
		tag = "HEAD"
	}
	cmd := exec.Command("git", "ls-remote", repo, tag)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "error getting remote HEAD: %s", string(out))
	}
	return nil
}

func parseGitURL(source string) (repo, tag string) {
	parts := strings.SplitN(source, "#", 2)
	repo = parts[0]
	if len(parts) > 1 {
		tag = parts[1]
	}
	return
}
