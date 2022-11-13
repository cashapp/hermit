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
	etag, err := util.GitClone(b, &util.RealCommandRunner{}, s.URL, checkoutDir)
	if err != nil {
		return "", "", "", errors.Wrap(err, s.URL)
	}
	return filepath.Join(cache.root, base), etag, "", nil
}

func (s *gitSource) ETag(b *ui.Task) (etag string, err error) {
	repo, tag := util.ParseGitURL(s.URL)
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
	repo, tag := util.ParseGitURL(s.URL)
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
