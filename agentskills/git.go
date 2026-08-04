package agentskills

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/otiai10/copy"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util/flock"
)

// refStamp records the last successfully resolved remote HEAD for a
// repository URL, so activation only pays for a network round trip when the
// freshness window has lapsed, and offline activation can fall back to the
// last good resolution.
type refStamp struct {
	URL       string    `json:"url"`
	SHA       string    `json:"sha"`
	CheckedAt time.Time `json:"checked_at"`
}

// ensureSnapshots resolves the repository to a commit and materialises a
// snapshot for each declared skill, returning skill name -> snapshot dir.
func ensureSnapshots(l *ui.UI, root string, repo SkillRepo) (map[string]string, error) {
	sha := repo.Ref
	if sha == "" {
		var err error
		sha, err = resolveHead(l, root, repo.URL)
		if err != nil {
			return nil, err
		}
	}

	snapshotsDir := filepath.Join(root, "snapshots")
	result := map[string]string{}
	var missing []string
	for _, name := range repo.Skills {
		dir := filepath.Join(snapshotsDir, snapshotDirName(name, sha))
		result[name] = dir
		if _, err := os.Stat(dir); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return result, nil
	}

	release, err := lockState(root)
	if err != nil {
		return nil, err
	}
	defer release() //nolint:errcheck

	// Re-check under the lock: another activation may have materialised the
	// snapshots while we waited.
	missing = missing[:0]
	for _, name := range repo.Skills {
		if _, err := os.Stat(result[name]); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return result, nil
	}

	task := l.Task("skills")
	defer task.Done()
	checkout, cleanup, err := fetchCommit(task, root, repo.URL, sha)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	for _, name := range missing {
		src := filepath.Join(checkout, filepath.FromSlash(repo.Path), name)
		if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
			return nil, errors.Errorf("skill %q not found at %s in %s@%s", name, filepath.Join(repo.Path, name), repo.URL, sha[:12])
		}
		if _, err := os.Stat(filepath.Join(src, "SKILL.md")); err != nil {
			return nil, errors.Errorf("skill %q in %s@%s has no SKILL.md", name, repo.URL, sha[:12])
		}
		if err := snapshot(src, snapshotsDir, snapshotDirName(name, sha)); err != nil {
			return nil, errors.Wrapf(err, "snapshotting skill %q", name)
		}
		task.Infof("Installed agent skill %s@%s", name, sha[:12])
	}
	return result, nil
}

// resolveHead returns the commit SHA of the remote HEAD, consulting the
// freshness stamp first and falling back to it when the remote is
// unreachable.
func resolveHead(l *ui.UI, root, url string) (string, error) {
	stampPath := filepath.Join(root, "refs", hashKey(url)+".json")
	stamp, _ := readStamp(stampPath)
	if stamp != nil && time.Since(stamp.CheckedAt) < freshnessWindow {
		return stamp.SHA, nil
	}

	sha, err := lsRemoteHead(url)
	if err != nil {
		if stamp != nil {
			l.Warnf("skills: could not check %s for updates, using last known revision %s: %s", url, stamp.SHA[:12], err)
			return stamp.SHA, nil
		}
		return "", errors.Wrapf(err, "could not resolve HEAD (offline and no previous snapshot?)")
	}
	writeStamp(stampPath, &refStamp{URL: url, SHA: sha, CheckedAt: time.Now()})
	return sha, nil
}

func lsRemoteHead(url string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), resolveTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--", url, "HEAD")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		return "", errors.Wrapf(err, "git ls-remote %s", url)
	}
	fields := strings.Fields(string(out))
	if len(fields) < 1 || !commitSHARe.MatchString(fields[0]) {
		return "", errors.Errorf("unexpected ls-remote output from %s: %q", url, string(out))
	}
	return fields[0], nil
}

// fetchCommit produces a working checkout of the given commit in a transient
// directory. It first attempts a direct shallow fetch of the commit (works on
// GitHub and any server with allow-any-sha1-in-want) and falls back to a
// shallow clone of the default branch.
func fetchCommit(task *ui.Task, root, url, sha string) (dir string, cleanup func(), err error) {
	tmpRoot := filepath.Join(root, "tmp")
	if err := os.MkdirAll(tmpRoot, 0700); err != nil {
		return "", nil, errors.WithStack(err)
	}
	tmp, err := os.MkdirTemp(tmpRoot, "checkout-*")
	if err != nil {
		return "", nil, errors.WithStack(err)
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }

	err = runGit(task, tmp, "init", "--quiet", ".")
	if err == nil {
		err = runGit(task, tmp, "fetch", "--quiet", "--depth=1", "--", url, sha)
		if err == nil {
			err = runGit(task, tmp, "checkout", "--quiet", "--detach", "FETCH_HEAD")
		}
	}
	if err == nil {
		return tmp, cleanup, nil
	}

	// Fallback for servers that refuse fetching arbitrary SHAs.
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0700); err != nil {
		return "", nil, errors.WithStack(err)
	}
	if err := runGit(task, tmpRoot, "clone", "--quiet", "--depth=1", "--", url, tmp); err != nil {
		cleanup()
		return "", nil, errors.Wrapf(err, "fetching %s", url)
	}
	out, err := gitOutput(tmp, "rev-parse", "HEAD")
	if err != nil {
		cleanup()
		return "", nil, err
	}
	if head := strings.TrimSpace(out); head != sha {
		cleanup()
		return "", nil, errors.Errorf("%s: default branch moved to %s while resolving %s and the server does not support fetching commits directly", url, head[:12], sha[:12])
	}
	return tmp, cleanup, nil
}

func runGit(task *ui.Task, dir string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	_, _ = task.Write(out)
	return nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...) //nolint:noctx
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", errors.Wrapf(err, "git %s", strings.Join(args, " "))
	}
	return string(out), nil
}

// snapshot copies the skill tree into the snapshots directory via a temporary
// directory and atomic rename, so a partially written snapshot is never
// observable at its final path.
func snapshot(src, snapshotsDir, name string) error {
	if err := os.MkdirAll(snapshotsDir, 0700); err != nil {
		return errors.WithStack(err)
	}
	tmp, err := os.MkdirTemp(snapshotsDir, ".tmp-"+name+"-*")
	if err != nil {
		return errors.WithStack(err)
	}
	defer os.RemoveAll(tmp)
	// Skip symlinks: a skill snapshot must be self-contained and a
	// repository symlink could point anywhere.
	err = copy.Copy(src, tmp, copy.Options{
		OnSymlink: func(string) copy.SymlinkAction { return copy.Skip },
	})
	if err != nil {
		return errors.WithStack(err)
	}
	dest := filepath.Join(snapshotsDir, name)
	if err := os.Rename(tmp, dest); err != nil {
		if os.IsExist(err) || errors.Is(err, os.ErrExist) {
			return nil // Lost a benign race with another process.
		}
		return errors.WithStack(err)
	}
	return nil
}

func lockState(root string) (release func() error, err error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, errors.WithStack(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	release, err = flock.Acquire(ctx, filepath.Join(root, ".lock"), "materialising agent skills")
	cancel()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return release, nil
}

func readStamp(path string) (*refStamp, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	stamp := &refStamp{}
	if err := json.Unmarshal(data, stamp); err != nil {
		return nil, errors.WithStack(err)
	}
	if !commitSHARe.MatchString(stamp.SHA) {
		return nil, errors.Errorf("corrupt ref stamp %s", path)
	}
	return stamp, nil
}

func writeStamp(path string, stamp *refStamp) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	data, err := json.Marshal(stamp)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
