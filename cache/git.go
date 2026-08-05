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
	repo, tag, err := parseGitURL(s.URL)
	if err != nil {
		return "", "", "", err
	}
	if isFullGitSHA(tag) {
		err = checkoutGitCommit(b, cache.root, repo, tag, checkoutDir)
	} else {
		args := []string{"git", "clone", "--depth=1"}
		if tag != "" {
			args = append(args, "--branch="+tag)
		}
		args = append(args, "--", repo, checkoutDir)
		err = util.RunInDir(b, cache.root, args...)
	}
	if err != nil {
		return "", "", "", errors.WithStack(err)
	}

	bts, err := util.CaptureInDir(b, checkoutDir, "git", "rev-parse", "HEAD")
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
	if isFullGitSHA(tag) {
		// A pinned commit is immutable, so no remote check is needed.
		return tag, nil
	}
	if tag == "" {
		tag = "HEAD"
	}
	bts, err := util.Capture(b, "git", "ls-remote", "--", repo, tag)
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
	repo, tag, err := parseGitURL(s.URL)
	if err != nil {
		return err
	}
	if tag == "" || isFullGitSHA(tag) {
		// A commit SHA cannot be listed with ls-remote, so just verify that
		// the repository is reachable.
		tag = "HEAD"
	}
	cmd := exec.Command("git", "ls-remote", "--", repo, tag) //nolint
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "error getting remote HEAD: %s", string(out))
	}
	return nil
}

// checkoutGitCommit fetches a single commit by SHA and checks it out.
//
// A commit SHA cannot be passed to "git clone --branch", so initialise an
// empty repository and fetch just the commit instead. This requires the
// server to allow fetching by commit SHA (GitHub and GitLab both do).
func checkoutGitCommit(b *ui.Task, root, repo, sha, checkoutDir string) error {
	if err := util.RunInDir(b, root, "git", "init", "--", checkoutDir); err != nil {
		return errors.WithStack(err)
	}
	if err := util.RunInDir(b, checkoutDir, "git", "fetch", "--depth=1", "--", repo, sha); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(util.RunInDir(b, checkoutDir, "git", "checkout", "--detach", "FETCH_HEAD"))
}

// isFullGitSHA reports whether ref is a full lowercase hex commit hash
// (40 characters for SHA-1, 64 for SHA-256).
func isFullGitSHA(ref string) bool {
	if len(ref) != 40 && len(ref) != 64 {
		return false
	}
	for _, r := range ref {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func parseGitURL(source string) (repo, tag string, err error) {
	parts := strings.SplitN(source, "#", 2)
	repo = parts[0]

	// Validate repo doesn't start with dash to prevent argument injection
	if strings.HasPrefix(repo, "-") {
		return "", "", errors.Errorf("invalid git URL: repository cannot start with '-': %s", repo)
	}

	if len(parts) > 1 {
		tag = parts[1]
	}
	return repo, tag, nil
}
