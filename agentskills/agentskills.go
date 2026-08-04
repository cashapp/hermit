// Package agentskills materialises agent skills declared in a Hermit
// environment's configuration and links them into the environment's
// .agents/skills and .claude/skills directories on activation.
//
// Skills are resolved to immutable, content-addressed snapshots under the
// Hermit state directory. The environment only ever contains symlinks into
// those snapshots; skill content is never written into the project itself.
package agentskills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
)

// SkillRepo configures a git repository providing agent skills.
type SkillRepo struct {
	URL    string   `hcl:"url,label" help:"Git repository URL providing agent skills."`
	Path   string   `hcl:"path,optional" help:"Subdirectory within the repository containing the skill directories."`
	Skills []string `hcl:"skills" help:"Names of the skill directories to link into the environment."`
	Ref    string   `hcl:"ref,optional" help:"Full commit SHA to pin to. When omitted the remote HEAD is used, re-checked at most every 15 minutes."`
}

const (
	// stateSubdir is the directory under the Hermit state dir holding all
	// agent skill state:
	//
	//   snapshots/<name>@<sha12>/  immutable skill content
	//   refs/<hash>.json           freshness stamps per repository URL
	//   ledgers/<hash>.json        per-environment link ledgers
	//   tmp/                       transient checkouts
	//   .lock                      global snapshot lock
	stateSubdir = "agent-skills"

	// freshnessWindow is how long a resolved remote HEAD is trusted before
	// it is re-checked on activation.
	freshnessWindow = 15 * time.Minute

	// resolveTimeout bounds remote git operations so offline activation
	// never blocks the shell for long.
	resolveTimeout = 10 * time.Second
)

var (
	skillNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	commitSHARe = regexp.MustCompile(`^[0-9a-f]{40}$`)

	// linkDirs are the environment-relative directories skills are linked
	// into. Both are ecosystem conventions for agent skill discovery.
	linkDirs = []string{
		filepath.Join(".agents", "skills"),
		filepath.Join(".claude", "skills"),
	}
)

// Validate checks the skill repository declarations for configuration errors.
func Validate(repos []SkillRepo) error {
	seen := map[string]string{}
	for _, repo := range repos {
		if repo.URL == "" {
			return errors.Errorf("skill-repo: repository URL is required")
		}
		if strings.HasPrefix(repo.URL, "-") {
			return errors.Errorf("skill-repo %q: invalid repository URL", repo.URL)
		}
		if len(repo.Skills) == 0 {
			return errors.Errorf("skill-repo %q: at least one skill name is required", repo.URL)
		}
		if repo.Ref != "" && !commitSHARe.MatchString(repo.Ref) {
			return errors.Errorf("skill-repo %q: ref must be a full 40-character commit SHA, got %q", repo.URL, repo.Ref)
		}
		if repo.Path != "" {
			clean := filepath.Clean(repo.Path)
			if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				return errors.Errorf("skill-repo %q: path must be relative to the repository root, got %q", repo.URL, repo.Path)
			}
		}
		for _, name := range repo.Skills {
			if !skillNameRe.MatchString(name) {
				return errors.Errorf("skill-repo %q: invalid skill name %q (must match %s)", repo.URL, name, skillNameRe)
			}
			if prev, ok := seen[name]; ok {
				return errors.Errorf("skill %q is declared by both %q and %q", name, prev, repo.URL)
			}
			seen[name] = repo.URL
		}
	}
	return nil
}

// Sync ensures every declared skill has an immutable snapshot under the state
// directory and that the environment's skill directories contain exactly the
// declared links. Fetch failures degrade to the last good snapshot with a
// warning; only configuration errors are fatal.
func Sync(l *ui.UI, stateDir, envRoot string, repos []SkillRepo) error {
	if err := Validate(repos); err != nil {
		return errors.WithStack(err)
	}
	root := filepath.Join(stateDir, stateSubdir)
	desired := map[string]string{} // skill name -> snapshot dir
	failed := map[string]bool{}    // skills whose repo could not be synced
	for _, repo := range repos {
		snapshots, err := ensureSnapshots(l, root, repo)
		if err != nil {
			l.Warnf("skills: %s: %s", repo.URL, err)
			// Leave any existing links for this repository's skills alone
			// rather than tearing down a previously working set.
			for _, name := range repo.Skills {
				failed[name] = true
			}
			continue
		}
		for name, dir := range snapshots {
			desired[name] = dir
		}
	}
	return errors.WithStack(reconcileLinks(l, root, envRoot, desired, failed))
}

func hashKey(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}

func snapshotDirName(name, sha string) string {
	return fmt.Sprintf("%s@%s", name, sha[:12])
}
