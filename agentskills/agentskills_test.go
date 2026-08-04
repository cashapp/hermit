package agentskills

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"

	"github.com/cashapp/hermit/ui"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, "git %v: %s", args, out)
	return string(out)
}

// makeSkillRepo creates a local git repository containing skill directories
// under skills/, returning its path and HEAD commit.
func makeSkillRepo(t *testing.T, skills ...string) (repoDir, head string) {
	t.Helper()
	repoDir = t.TempDir()
	git(t, repoDir, "init", "-q", "-b", "main", ".")
	for _, name := range skills {
		dir := filepath.Join(repoDir, "skills", name)
		assert.NoError(t, os.MkdirAll(dir, 0700))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+"\n"), 0600))
	}
	git(t, repoDir, "add", ".")
	git(t, repoDir, "commit", "-q", "-m", "skills")
	return repoDir, resolveTestHead(t, repoDir)
}

func resolveTestHead(t *testing.T, repoDir string) string {
	t.Helper()
	out := git(t, repoDir, "rev-parse", "HEAD")
	return out[:40]
}

func TestValidate(t *testing.T) {
	assert.NoError(t, Validate(nil))
	assert.NoError(t, Validate([]SkillRepo{{URL: "https://example.com/x.git", Skills: []string{"a-1", "b"}}}))

	assert.Error(t, Validate([]SkillRepo{{URL: "", Skills: []string{"a"}}}))
	assert.Error(t, Validate([]SkillRepo{{URL: "-upload-pack=x", Skills: []string{"a"}}}))
	assert.Error(t, Validate([]SkillRepo{{URL: "https://example.com/x.git"}}))
	assert.Error(t, Validate([]SkillRepo{{URL: "https://example.com/x.git", Skills: []string{"Bad_Name"}}}))
	assert.Error(t, Validate([]SkillRepo{{URL: "https://example.com/x.git", Skills: []string{"a"}, Ref: "abc123"}}))
	assert.Error(t, Validate([]SkillRepo{{URL: "https://example.com/x.git", Skills: []string{"a"}, Path: "../escape"}}))
	assert.Error(t, Validate([]SkillRepo{
		{URL: "https://example.com/x.git", Skills: []string{"a"}},
		{URL: "https://example.com/y.git", Skills: []string{"a"}},
	}))
}

func TestSyncLinksSkills(t *testing.T) {
	l, _ := ui.NewForTesting()
	repoDir, head := makeSkillRepo(t, "alpha", "beta")
	stateDir := t.TempDir()
	envRoot := t.TempDir()

	repos := []SkillRepo{{URL: repoDir, Path: "skills", Skills: []string{"alpha", "beta"}}}
	assert.NoError(t, Sync(l, stateDir, envRoot, repos))

	for _, dir := range []string{".agents/skills", ".claude/skills"} {
		for _, name := range []string{"alpha", "beta"} {
			link := filepath.Join(envRoot, dir, name)
			fi, err := os.Lstat(link)
			assert.NoError(t, err)
			assert.NotZero(t, fi.Mode()&os.ModeSymlink)
			target, err := os.Readlink(link)
			assert.NoError(t, err)
			assert.Equal(t, filepath.Join(stateDir, "agent-skills", "snapshots", name+"@"+head[:12]), target)
			_, err = os.Stat(filepath.Join(link, "SKILL.md"))
			assert.NoError(t, err)
		}
	}
}

func TestSyncRemovesUndeclaredLinks(t *testing.T) {
	l, _ := ui.NewForTesting()
	repoDir, _ := makeSkillRepo(t, "alpha", "beta")
	stateDir := t.TempDir()
	envRoot := t.TempDir()

	assert.NoError(t, Sync(l, stateDir, envRoot, []SkillRepo{{URL: repoDir, Path: "skills", Skills: []string{"alpha", "beta"}}}))
	assert.NoError(t, Sync(l, stateDir, envRoot, []SkillRepo{{URL: repoDir, Path: "skills", Skills: []string{"alpha"}}}))

	_, err := os.Lstat(filepath.Join(envRoot, ".agents", "skills", "alpha"))
	assert.NoError(t, err)
	_, err = os.Lstat(filepath.Join(envRoot, ".agents", "skills", "beta"))
	assert.Error(t, err)
	_, err = os.Lstat(filepath.Join(envRoot, ".claude", "skills", "beta"))
	assert.Error(t, err)
}

func TestSyncLeavesRepoCommittedSkillsAlone(t *testing.T) {
	l, _ := ui.NewForTesting()
	repoDir, _ := makeSkillRepo(t, "alpha")
	stateDir := t.TempDir()
	envRoot := t.TempDir()

	committed := filepath.Join(envRoot, ".agents", "skills", "alpha")
	assert.NoError(t, os.MkdirAll(committed, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(committed, "SKILL.md"), []byte("local"), 0600))

	assert.NoError(t, Sync(l, stateDir, envRoot, []SkillRepo{{URL: repoDir, Path: "skills", Skills: []string{"alpha"}}}))

	fi, err := os.Lstat(committed)
	assert.NoError(t, err)
	assert.True(t, fi.IsDir())
	data, err := os.ReadFile(filepath.Join(committed, "SKILL.md"))
	assert.NoError(t, err)
	assert.Equal(t, "local", string(data))
}

func TestSyncLeavesForeignSymlinksAlone(t *testing.T) {
	l, _ := ui.NewForTesting()
	repoDir, _ := makeSkillRepo(t, "alpha")
	stateDir := t.TempDir()
	envRoot := t.TempDir()

	other := t.TempDir()
	link := filepath.Join(envRoot, ".agents", "skills", "alpha")
	assert.NoError(t, os.MkdirAll(filepath.Dir(link), 0700))
	assert.NoError(t, os.Symlink(other, link))

	assert.NoError(t, Sync(l, stateDir, envRoot, []SkillRepo{{URL: repoDir, Path: "skills", Skills: []string{"alpha"}}}))

	target, err := os.Readlink(link)
	assert.NoError(t, err)
	assert.Equal(t, other, target)
}

func TestSyncPinnedRef(t *testing.T) {
	l, _ := ui.NewForTesting()
	repoDir, head := makeSkillRepo(t, "alpha")

	// Advance the repo past the pin.
	assert.NoError(t, os.WriteFile(filepath.Join(repoDir, "skills", "alpha", "extra.md"), []byte("new"), 0600))
	git(t, repoDir, "add", ".")
	git(t, repoDir, "commit", "-q", "-m", "update")

	stateDir := t.TempDir()
	envRoot := t.TempDir()
	assert.NoError(t, Sync(l, stateDir, envRoot, []SkillRepo{{URL: repoDir, Path: "skills", Skills: []string{"alpha"}, Ref: head}}))

	link := filepath.Join(envRoot, ".agents", "skills", "alpha")
	target, err := os.Readlink(link)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(stateDir, "agent-skills", "snapshots", "alpha@"+head[:12]), target)
	_, err = os.Stat(filepath.Join(link, "extra.md"))
	assert.Error(t, err)
}

func TestSyncUpdatesToNewHead(t *testing.T) {
	l, _ := ui.NewForTesting()
	repoDir, oldHead := makeSkillRepo(t, "alpha")
	stateDir := t.TempDir()
	envRoot := t.TempDir()
	repos := []SkillRepo{{URL: repoDir, Path: "skills", Skills: []string{"alpha"}}}

	assert.NoError(t, Sync(l, stateDir, envRoot, repos))

	assert.NoError(t, os.WriteFile(filepath.Join(repoDir, "skills", "alpha", "extra.md"), []byte("new"), 0600))
	git(t, repoDir, "add", ".")
	git(t, repoDir, "commit", "-q", "-m", "update")
	newHead := resolveTestHead(t, repoDir)
	assert.NotEqual(t, oldHead, newHead)

	// Expire the freshness stamp so the new HEAD is picked up.
	stampPath := filepath.Join(stateDir, "agent-skills", "refs", hashKey(repoDir)+".json")
	writeStamp(stampPath, &refStamp{URL: repoDir, SHA: oldHead, CheckedAt: time.Now().Add(-time.Hour)})

	assert.NoError(t, Sync(l, stateDir, envRoot, repos))

	target, err := os.Readlink(filepath.Join(envRoot, ".agents", "skills", "alpha"))
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(stateDir, "agent-skills", "snapshots", "alpha@"+newHead[:12]), target)
	_, err = os.Stat(filepath.Join(envRoot, ".agents", "skills", "alpha", "extra.md"))
	assert.NoError(t, err)
}

func TestSyncFreshnessWindowSkipsRemoteCheck(t *testing.T) {
	l, _ := ui.NewForTesting()
	repoDir, head := makeSkillRepo(t, "alpha")
	stateDir := t.TempDir()
	envRoot := t.TempDir()
	repos := []SkillRepo{{URL: repoDir, Path: "skills", Skills: []string{"alpha"}}}

	assert.NoError(t, Sync(l, stateDir, envRoot, repos))

	// Advance the repo; within the freshness window Sync must keep the
	// stamped revision.
	assert.NoError(t, os.WriteFile(filepath.Join(repoDir, "skills", "alpha", "extra.md"), []byte("new"), 0600))
	git(t, repoDir, "add", ".")
	git(t, repoDir, "commit", "-q", "-m", "update")

	assert.NoError(t, Sync(l, stateDir, envRoot, repos))
	target, err := os.Readlink(filepath.Join(envRoot, ".agents", "skills", "alpha"))
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(stateDir, "agent-skills", "snapshots", "alpha@"+head[:12]), target)
}

func TestSyncOfflineFallsBackToLastGood(t *testing.T) {
	l, _ := ui.NewForTesting()
	repoDir, head := makeSkillRepo(t, "alpha")
	stateDir := t.TempDir()
	envRoot := t.TempDir()
	repos := []SkillRepo{{URL: repoDir, Path: "skills", Skills: []string{"alpha"}}}

	assert.NoError(t, Sync(l, stateDir, envRoot, repos))

	// Simulate the remote disappearing with an expired stamp: resolution
	// fails but the last good snapshot keeps working.
	assert.NoError(t, os.RemoveAll(filepath.Join(repoDir, ".git")))
	stampPath := filepath.Join(stateDir, "agent-skills", "refs", hashKey(repoDir)+".json")
	writeStamp(stampPath, &refStamp{URL: repoDir, SHA: head, CheckedAt: time.Now().Add(-time.Hour)})

	assert.NoError(t, Sync(l, stateDir, envRoot, repos))
	target, err := os.Readlink(filepath.Join(envRoot, ".agents", "skills", "alpha"))
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(stateDir, "agent-skills", "snapshots", "alpha@"+head[:12]), target)
}

func TestSyncEmptyConfigCleansUp(t *testing.T) {
	l, _ := ui.NewForTesting()
	repoDir, _ := makeSkillRepo(t, "alpha")
	stateDir := t.TempDir()
	envRoot := t.TempDir()

	assert.NoError(t, Sync(l, stateDir, envRoot, []SkillRepo{{URL: repoDir, Path: "skills", Skills: []string{"alpha"}}}))
	assert.NoError(t, Sync(l, stateDir, envRoot, nil))

	_, err := os.Lstat(filepath.Join(envRoot, ".agents", "skills", "alpha"))
	assert.Error(t, err)
	_, err = os.Lstat(filepath.Join(envRoot, ".claude", "skills", "alpha"))
	assert.Error(t, err)
}

func TestSyncMissingSkillWarnsButLinksRest(t *testing.T) {
	l, _ := ui.NewForTesting()
	repoDir, _ := makeSkillRepo(t, "alpha")
	stateDir := t.TempDir()
	envRoot := t.TempDir()

	// "ghost" is declared but does not exist in the repository: the repo
	// fails with a warning and contributes nothing, without failing Sync.
	repos := []SkillRepo{{URL: repoDir, Path: "skills", Skills: []string{"alpha", "ghost"}}}
	assert.NoError(t, Sync(l, stateDir, envRoot, repos))

	_, err := os.Lstat(filepath.Join(envRoot, ".agents", "skills", "alpha"))
	assert.Error(t, err)
	_, err = os.Lstat(filepath.Join(envRoot, ".agents", "skills", "ghost"))
	assert.Error(t, err)
}

func TestSyncFailedRepoPreservesExistingLinks(t *testing.T) {
	l, _ := ui.NewForTesting()
	repoDir, _ := makeSkillRepo(t, "alpha")
	stateDir := t.TempDir()
	envRoot := t.TempDir()

	assert.NoError(t, Sync(l, stateDir, envRoot, []SkillRepo{{URL: repoDir, Path: "skills", Skills: []string{"alpha"}}}))

	// Declaring a skill the repository does not contain makes the whole
	// repository fail to sync; the previously linked skill must survive.
	assert.NoError(t, Sync(l, stateDir, envRoot, []SkillRepo{{URL: repoDir, Path: "skills", Skills: []string{"alpha", "ghost"}}}))

	_, err := os.Lstat(filepath.Join(envRoot, ".agents", "skills", "alpha"))
	assert.NoError(t, err)
	_, err = os.Lstat(filepath.Join(envRoot, ".claude", "skills", "alpha"))
	assert.NoError(t, err)
}
