package cache

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/redact"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

type gitSource struct {
	URL    redact.URL
	runner util.CommandRunner
}

func (s *gitSource) OpenLocal(c *Cache, checksum string) (*os.File, error) {
	f, err := os.Open(c.Path(checksum, s.URL))
	return f, errors.WithStack(err)
}

func (s *gitSource) Download(b *ui.Task, cache *Cache, checksum string) (string, string, string, error) {
	base := BasePath(checksum, s.URL)
	checkoutDir := filepath.Join(cache.root, base)
	repo, tag, err := parseGitURL(s.URL)
	if err != nil {
		return "", "", "", err
	}
	args := redact.Args(util.GitArgs("clone", "--depth=1")...)
	if tag != "" {
		args = append(args, redact.Plain("--branch="+tag))
	}
	args = append(args, redact.Plain("--"), repo, redact.Plain(checkoutDir))
	err = s.runner.RunInDir(b, cache.root, args...)
	if err != nil {
		return "", "", "", errors.WithStack(err)
	}

	bts, err := s.runner.CaptureInDir(b, checkoutDir, redact.Args("git", "rev-parse", "HEAD")...)
	if err != nil {
		return "", "", "", errors.WithStack(err)
	}
	etag := strings.Trim(string(bts), "\n")

	return filepath.Join(cache.root, base), etag, "", nil
}

func (s *gitSource) ETag(b *ui.Task) (etag string, err error) {
	bts, err := s.lsRemote(b)
	if err != nil {
		return "", errors.Wrap(err, s.URL.String())
	}
	str := string(bts)
	parts := strings.Split(str, "\t")
	if len(parts) != 2 {
		return "", errors.Errorf("invalid HEAD: %s", str)
	}

	return parts[0], nil
}

func (s *gitSource) Validate() error {
	_, err := s.lsRemote(nil)
	return errors.Wrap(err, "error getting remote HEAD")
}

func (s *gitSource) lsRemote(log ui.Logger) ([]byte, error) {
	repo, tag, err := parseGitURL(s.URL)
	if err != nil {
		return nil, err
	}
	if tag == "" {
		tag = "HEAD"
	}
	args := append(redact.Args(util.GitArgs("ls-remote", "--")...), repo, redact.Plain(tag))
	return s.runner.CaptureInDir(log, "", args...)
}

func parseGitURL(source redact.URL) (repo redact.URL, tag string, err error) {
	parts := strings.SplitN(source.Reveal(), "#", 2)
	repo = redact.URL(parts[0])

	if err := util.ValidateGitURL(repo); err != nil {
		return "", "", errors.WithStack(err)
	}

	if len(parts) > 1 {
		tag = parts[1]
	}
	return repo, tag, nil
}
