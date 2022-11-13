package util

import (
	"strings"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
)

// GitClone clones a git repository and optionally checks out a ref, if specified (via <url>#<ref>).
func GitClone(task *ui.Task, runner CommandRunner, url, checkoutDir string) (head string, err error) {
	repo, ref := ParseGitURL(url)
	args := []string{"git", "clone"}
	if ref == "" {
		args = append(args, "--depth=1")
	}
	args = append(args, repo, checkoutDir)
	err = runner.RunInDir(task, ".", args...)
	if err != nil {
		return "", errors.WithStack(err)
	}
	if ref != "" {
		task.Infof("%s: checking out %s", url, ref)
		err = runner.RunInDir(task, checkoutDir, "git", "reset", "--hard", ref)
		if err != nil {
			return "", errors.WithStack(err)
		}
	}
	bts, err := runner.CaptureInDir(task, checkoutDir, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", errors.WithStack(err)
	}
	return strings.Trim(string(bts), "\n"), nil
}

// ParseGitURL into a repo and an optional #ref.
func ParseGitURL(source string) (repo, ref string) {
	parts := strings.SplitN(source, "#", 2)
	repo = parts[0]
	if len(parts) > 1 {
		ref = parts[1]
	}
	return
}
