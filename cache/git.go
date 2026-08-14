package cache

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/internal/redact"
	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

type gitSource struct {
	URL sources.Source
}

func (s *gitSource) OpenLocal(c *Cache, checksum string) (*os.File, error) {
	f, err := os.Open(c.path(checksum, s.URL))
	return f, errors.WithStack(err)
}

func (s *gitSource) Download(b *ui.Task, cache *Cache, checksum string) (string, string, string, error) {
	base := basePath(checksum, s.URL)
	checkoutDir := filepath.Join(cache.root, base)
	repo, tag, err := parseGitURL(s.URL)
	if err != nil {
		return "", "", "", err
	}
	args := util.GitArgs("clone", "--depth=1")
	if tag != "" {
		args = append(args, "--branch="+tag)
	}
	args = append(args, "--")
	err = util.RunSystemInDirWithSource(b, cache.root, args, repo, checkoutDir)
	if err != nil {
		return "", "", "", errors.WithStack(err)
	}

	bts, err := util.CaptureSystemInDir(b, checkoutDir, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", "", "", errors.WithStack(err)
	}
	etag := strings.Trim(string(bts), "\n")

	return filepath.Join(cache.root, base), etag, "", nil
}

func (s *gitSource) ETag(b *ui.Task) (etag string, err error) {
	repo, tag, err := parseGitURL(s.URL)
	if err != nil {
		return "", err
	}
	if tag == "" {
		tag = "HEAD"
	}
	bts, err := util.CaptureSystemWithSource(b, util.GitArgs("ls-remote", "--"), repo, tag)
	if err != nil {
		return "", errors.Wrap(err, s.URL.String())
	}
	str := string(bts)
	parts := strings.Split(str, "\t")
	if len(parts) != 2 {
		return "", errors.Errorf("invalid HEAD: %s", redact.Credentials(str))
	}

	return parts[0], nil
}

func (s *gitSource) Validate() error {
	repo, tag, err := parseGitURL(s.URL)
	if err != nil {
		return err
	}
	if tag == "" {
		tag = "HEAD"
	}
	args := util.GitArgs("ls-remote", "--")
	cmd, err := util.SystemCommandWithSource(args, repo, tag)
	if err != nil {
		return errors.WithStack(err)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "error getting remote HEAD: %s", redact.Credentials(string(out)))
	}
	return nil
}

func parseGitURL(source sources.Source) (repo sources.Source, tag string, err error) {
	parts := strings.SplitN(source.Get(), "#", 2)
	repo = sources.NewSource(parts[0])

	if err := util.ValidateGitURL(repo); err != nil {
		return sources.Source{}, "", errors.WithStack(err)
	}

	if len(parts) > 1 {
		tag = parts[1]
	}
	return repo, tag, nil
}
