package agentskills

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
)

// ledger records the symlinks this machine created for an environment, so
// reconciliation can safely remove links for skills that are no longer
// declared without ever touching nodes it does not own.
type ledger struct {
	EnvRoot string        `json:"env_root"`
	Links   []ledgerEntry `json:"links"`
}

type ledgerEntry struct {
	// Link is the symlink path relative to the environment root.
	Link string `json:"link"`
	// Target is the absolute snapshot path the link pointed at.
	Target string `json:"target"`
}

// reconcileLinks makes the environment's skill directories match the desired
// skill set: creating missing links, retargeting stale ones we own, and
// removing links for undeclared skills. Pre-existing nodes not created by us
// (e.g. a repo-committed skill directory) always win and are left untouched.
// Skills in failed are still declared but could not be synced; their existing
// links are preserved as-is.
func reconcileLinks(l *ui.UI, root, envRoot string, desired map[string]string, failed map[string]bool) error {
	ledgerPath := filepath.Join(root, "ledgers", hashKey(envRoot)+".json")
	previous := readLedger(ledgerPath)
	snapshotsDir := filepath.Join(root, "snapshots")

	owned := map[string]string{}
	for _, entry := range previous.Links {
		owned[entry.Link] = entry.Target
	}

	next := &ledger{EnvRoot: envRoot}
	created := false
	for _, dir := range linkDirs {
		for name, target := range desired {
			rel := filepath.Join(dir, name)
			link := filepath.Join(envRoot, rel)
			madeNew, err := ensureLink(l, link, rel, target, snapshotsDir, owned[rel])
			if err != nil {
				return err
			}
			created = created || madeNew
			next.Links = append(next.Links, ledgerEntry{Link: rel, Target: target})
		}
	}

	// Remove links we created for skills that are no longer declared.
	for _, entry := range previous.Links {
		name := filepath.Base(entry.Link)
		if _, still := desired[name]; still {
			continue
		}
		if failed[name] {
			// Still declared, just unsyncable right now: keep both the link
			// and its ledger record.
			next.Links = append(next.Links, entry)
			continue
		}
		link := filepath.Join(envRoot, entry.Link)
		fi, err := os.Lstat(link)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			continue
		}
		dest, err := os.Readlink(link)
		if err != nil || (dest != entry.Target && !within(dest, snapshotsDir)) {
			continue
		}
		if err := os.Remove(link); err != nil {
			l.Warnf("skills: could not remove stale link %s: %s", link, err)
		}
	}

	if len(next.Links) == 0 && len(previous.Links) == 0 {
		return nil
	}
	sort.Slice(next.Links, func(i, j int) bool { return next.Links[i].Link < next.Links[j].Link })
	writeLedger(ledgerPath, next)
	if created {
		warnIfNotIgnored(l, envRoot)
	}
	return nil
}

// ensureLink points link at target, creating it atomically. It only replaces
// an existing node when it is a symlink we own: one recorded in the ledger or
// pointing into the snapshots directory.
func ensureLink(l *ui.UI, link, rel, target, snapshotsDir, ownedTarget string) (created bool, err error) {
	fi, lerr := os.Lstat(link)
	switch {
	case lerr == nil && fi.Mode()&os.ModeSymlink != 0:
		dest, err := os.Readlink(link)
		if err == nil && dest == target {
			return false, nil
		}
		if dest != ownedTarget && !within(dest, snapshotsDir) {
			l.Warnf("skills: %s is a symlink not managed by Hermit, leaving it alone", rel)
			return false, nil
		}
	case lerr == nil:
		// A real file or directory: the repository provides this skill
		// itself and it takes precedence.
		l.Debugf("skills: %s exists in the repository, leaving it alone", rel)
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(link), 0750); err != nil {
		return false, errors.WithStack(err)
	}
	tmp := link + ".hermit-tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return false, errors.WithStack(err)
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return false, errors.WithStack(err)
	}
	return lerr != nil, nil
}

func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// warnIfNotIgnored nudges the user to gitignore the skill link directories.
// The links contain machine-local absolute paths and must not be committed.
func warnIfNotIgnored(l *ui.UI, envRoot string) {
	if _, err := os.Stat(filepath.Join(envRoot, ".git")); err != nil {
		return
	}
	probe := filepath.Join(linkDirs[0], "probe")
	cmd := exec.Command("git", "-C", envRoot, "check-ignore", "-q", "--", probe) //nolint:noctx
	if err := cmd.Run(); err != nil {
		l.Warnf("skills: add %q and %q to .gitignore — Hermit-managed skill links must not be committed", linkDirs[0]+string(filepath.Separator), linkDirs[1]+string(filepath.Separator))
	}
}

func readLedger(path string) *ledger {
	led := &ledger{}
	data, err := os.ReadFile(path)
	if err != nil {
		return led
	}
	_ = json.Unmarshal(data, led)
	return led
}

func writeLedger(path string, led *ledger) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	data, err := json.MarshalIndent(led, "", "  ")
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
