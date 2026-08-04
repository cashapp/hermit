package sources_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"

	"github.com/cashapp/hermit/sources"
	"github.com/cashapp/hermit/ui"
	"github.com/cashapp/hermit/util"
)

// slowCloningGit is a fake util.CommandRunner that behaves like a real "git"
// binary just enough to exercise GitSource.Sync: "clone" sleeps for a bit
// (simulating a slow network fetch) before writing a manifest and a ".git"
// marker into dest, and "pull" is an instant no-op success. Every clone
// appends a line to a shared log file, so tests can assert exactly one clone
// happened even under concurrent Sync calls.
type slowCloningGit struct {
	cloneDelay time.Duration
	cloneLog   string

	mu        sync.Mutex
	logHandle *os.File
}

func newSlowCloningGit(cloneDelay time.Duration, cloneLog string) *slowCloningGit {
	return &slowCloningGit{cloneDelay: cloneDelay, cloneLog: cloneLog}
}

func (g *slowCloningGit) RunInDir(_ *ui.Task, dir string, args ...string) error {
	if len(args) < 2 || args[0] != "git" {
		return fmt.Errorf("unexpected command: %v", args)
	}
	switch args[1] {
	case "pull":
		return nil
	case "clone":
		time.Sleep(g.cloneDelay)
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/master\n"), 0600); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "pkg.hcl"), []byte("description = \"test\"\n"), 0600); err != nil {
			return err
		}
		return g.appendLog(dir)
	default:
		return fmt.Errorf("unexpected git subcommand: %v", args)
	}
}

func (g *slowCloningGit) appendLog(dir string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.logHandle == nil {
		f, err := os.OpenFile(g.cloneLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		g.logHandle = f
	}
	_, err := fmt.Fprintf(g.logHandle, "%s\n", dir)
	return err
}

// pollForVanishAfterAppearing polls path until stop is closed, and reports
// an error on errCh if it ever observes path disappear after having
// previously observed it exist. This is the direct reproduction of the
// reported symptom: a reader elsewhere in the system seeing a manifest it
// already found go missing mid-sync.
func pollForVanishAfterAppearing(stop <-chan struct{}, path string, errCh chan<- error) {
	var sawIt bool
	for {
		select {
		case <-stop:
			errCh <- nil
			return
		default:
		}
		_, err := os.ReadFile(path)
		switch {
		case err == nil:
			sawIt = true
		case os.IsNotExist(err):
			if sawIt {
				errCh <- fmt.Errorf("manifest disappeared after first appearing: %s", path)
				return
			}
		default:
			errCh <- err
			return
		}
		time.Sleep(200 * time.Microsecond)
	}
}

// waitForFile polls for path to exist, failing the test if timeout elapses
// first. Used to synchronise on a real cross-process event (e.g. "the child
// has acquired the lock") without a fixed sleep, which would either race or
// needlessly slow the test down.
func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s to appear", timeout, path)
		}
		time.Sleep(time.Millisecond)
	}
}

// assertOneClone asserts exactly one clone occurred, ie. there was no
// thundering herd of redundant clones once the lock and double-checked
// locking are in place.
func assertOneClone(t *testing.T, cloneLog string) {
	t.Helper()
	data, err := os.ReadFile(cloneLog)
	assert.NoError(t, err)
	lines := strings.TrimSpace(string(data))
	if lines == "" {
		t.Fatalf("expected exactly one clone, got none")
	}
	got := strings.Split(lines, "\n")
	assert.Equal(t, 1, len(got))
}

// assertNoScratchDirs asserts no leaked ".tmp-*"/"*.old" scratch directories
// remain in sourceDir after all syncs have completed.
func assertNoScratchDirs(t *testing.T, sourceDir string) {
	t.Helper()
	entries, err := os.ReadDir(sourceDir)
	assert.NoError(t, err)
	for _, entry := range entries {
		name := entry.Name()
		if strings.Contains(name, ".tmp-") || strings.HasSuffix(name, ".old") {
			t.Fatalf("leaked scratch directory: %s", name)
		}
	}
}

// TestConcurrentSyncInProcess reproduces N goroutines racing to Sync the
// same not-yet-cloned source, each building its own *GitSource sharing one
// sourceDir and URI -- mirroring how each Hermit invocation constructs its
// own Sources from scratch. A background reader polls the manifest file
// GitSource.Sync writes and fails the test if it ever sees the manifest
// vanish after having first seen it exist.
//
// This exercises whatever process-local synchronisation sources/lock.go adds
// (hence -race), but not flock's cross-process behaviour -- see
// TestConcurrentSyncAcrossProcesses for that. On an unfixed tree, this test
// is not guaranteed to reproduce the race on every run, since all n
// goroutines racing through Sync at once is a timing-dependent condition,
// not a certainty -- see the start barrier below, which maximises the odds
// by holding every goroutine at the gate until all are spawned.
func TestConcurrentSyncInProcess(t *testing.T) {
	const n = 8
	sourceDir := t.TempDir()
	cloneLog := filepath.Join(t.TempDir(), "clones.log")
	uri := "git://concurrent-test"
	runner := newSlowCloningGit(50*time.Millisecond, cloneLog)
	manifestPath := filepath.Join(sourceDir, util.Hash(uri), "pkg.hcl")

	stop := make(chan struct{})
	readerErr := make(chan error, 1)
	go pollForVanishAfterAppearing(stop, manifestPath, readerErr)

	var ready sync.WaitGroup
	start := make(chan struct{})
	var wg sync.WaitGroup
	ready.Add(n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready.Done()
			<-start
			u, _ := ui.NewForTesting()
			source := sources.NewGitSource(uri, sourceDir, runner)
			_, err := source.Sync(u, true)
			assert.NoError(t, err)
		}()
	}
	ready.Wait()
	close(start)
	wg.Wait()
	close(stop)
	assert.NoError(t, <-readerErr)

	_, err := os.Stat(manifestPath)
	assert.NoError(t, err)
	assertOneClone(t, cloneLog)
	assertNoScratchDirs(t, sourceDir)
}

// TestConcurrentSyncAcrossProcesses is the reproducer for the reported bug:
// N genuinely separate processes race to Sync the same not-yet-cloned
// source. util/flock is deliberately re-entrant per-PID (a lock file
// recording our own PID is treated as already held), so this race can only
// be reproduced -- and the fix only validated -- across real processes, not
// goroutines sharing a PID.
//
// Each child is a re-exec of this same test binary running
// TestSyncChildProcess, guarded by HERMIT_TEST_CHILD so it no-ops otherwise.
func TestConcurrentSyncAcrossProcesses(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns subprocesses; skipped in -short")
	}
	const n = 8
	sourceDir := t.TempDir()
	cloneLog := filepath.Join(t.TempDir(), "clones.log")
	readyDir := t.TempDir()
	goFile := filepath.Join(t.TempDir(), "go")
	uri := "git://concurrent-cross-process-test"
	manifestPath := filepath.Join(sourceDir, util.Hash(uri), "pkg.hcl")

	stop := make(chan struct{})
	readerErr := make(chan error, 1)
	go pollForVanishAfterAppearing(stop, manifestPath, readerErr)

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			readyFile := filepath.Join(readyDir, fmt.Sprintf("%d", i))
			cmd := exec.Command(os.Args[0], "-test.run=TestSyncChildProcess", "-test.v")
			cmd.Env = append(os.Environ(),
				"HERMIT_TEST_CHILD=1",
				"HERMIT_TEST_SOURCE_URI="+uri,
				"HERMIT_TEST_SOURCE_DIR="+sourceDir,
				"HERMIT_TEST_CLONE_LOG="+cloneLog,
				"HERMIT_TEST_CLONE_DELAY=100ms",
				"HERMIT_TEST_READY_FILE="+readyFile,
				"HERMIT_TEST_GO_FILE="+goFile,
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				errs[i] = fmt.Errorf("child %d failed: %w\n%s", i, err, out)
			}
		}(i)
	}

	// Hold every child at the gate (each blocked on its own readyFile
	// existing, then waiting on goFile) until all n have signalled ready,
	// then release them all at once -- this maximises the odds that all n
	// children race through Sync concurrently, same as the in-process
	// start barrier above.
	for i := range n {
		waitForFile(t, filepath.Join(readyDir, fmt.Sprintf("%d", i)), 30*time.Second)
	}
	assert.NoError(t, os.WriteFile(goFile, nil, 0600))

	wg.Wait()
	close(stop)
	assert.NoError(t, <-readerErr)

	for _, e := range errs {
		assert.NoError(t, e)
	}
	assertOneClone(t, cloneLog)
	assertNoScratchDirs(t, sourceDir)
}

// TestSyncChildProcess is not a real test: it's the worker spawned by
// TestConcurrentSyncAcrossProcesses via re-exec, guarded by an env var so it
// no-ops under a normal test run.
func TestSyncChildProcess(t *testing.T) {
	if os.Getenv("HERMIT_TEST_CHILD") == "" {
		t.Skip("only runs as a spawned child of TestConcurrentSyncAcrossProcesses")
	}
	uri := os.Getenv("HERMIT_TEST_SOURCE_URI")
	sourceDir := os.Getenv("HERMIT_TEST_SOURCE_DIR")
	cloneLog := os.Getenv("HERMIT_TEST_CLONE_LOG")
	delay, err := time.ParseDuration(os.Getenv("HERMIT_TEST_CLONE_DELAY"))
	if err != nil {
		t.Fatalf("bad clone delay: %s", err)
	}

	// Signal the parent we're up, then wait for its go-ahead: this holds all
	// n children at the gate so they race through Sync together, rather than
	// however staggered process spawn happens to make them.
	if readyFile := os.Getenv("HERMIT_TEST_READY_FILE"); readyFile != "" {
		if err := os.WriteFile(readyFile, nil, 0600); err != nil {
			t.Fatalf("failed to write ready file: %s", err)
		}
		waitForFile(t, os.Getenv("HERMIT_TEST_GO_FILE"), 30*time.Second)
	}

	runner := newSlowCloningGit(delay, cloneLog)
	source := sources.NewGitSource(uri, sourceDir, runner)
	u, _ := ui.NewForTesting()
	if _, err := source.Sync(u, true); err != nil {
		t.Fatalf("sync failed: %s", err)
	}
}

// TestSyncLockTimeoutFallsBackToExistingCopy verifies that, when a source
// already has a usable copy on disk but the sync lock can't be acquired
// within the configured timeout, Sync degrades to using the existing copy
// rather than failing outright. Lock contention is exercised with a genuine
// separate process (TestHoldSourceLockChildProcess), for the same
// per-PID-reentrancy reason as the cross-process clone test above.
func TestSyncLockTimeoutFallsBackToExistingCopy(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a subprocess; skipped in -short")
	}
	sourceDir := t.TempDir()
	uri := "git://lock-timeout-test"
	runner := newSlowCloningGit(0, filepath.Join(t.TempDir(), "clones.log"))
	source := sources.NewGitSource(uri, sourceDir, runner)

	// Populate an initial copy so Sync has an existing tree to fall back to.
	u, _ := ui.NewForTesting()
	_, err := source.Sync(u, true)
	assert.NoError(t, err)

	path := filepath.Join(sourceDir, util.Hash(uri))
	readyFile := filepath.Join(t.TempDir(), "lock-held")
	holdFor := 500 * time.Millisecond
	cmd := exec.Command(os.Args[0], "-test.run=TestHoldSourceLockChildProcess", "-test.v")
	cmd.Env = append(os.Environ(),
		"HERMIT_TEST_CHILD=1",
		"HERMIT_TEST_LOCK_PATH="+path,
		"HERMIT_TEST_LOCK_HOLD="+holdFor.String(),
		"HERMIT_TEST_LOCK_READY_FILE="+readyFile,
	)
	assert.NoError(t, cmd.Start())
	// Guarantee the child is reaped even if an assertion below fails the
	// test early (assert.* calls t.Fatalf, which skips the cmd.Wait() at
	// the end of this function): an orphaned child would otherwise keep
	// holding the lock file open for the rest of holdFor.
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// Wait for the child to actually confirm it holds the lock, rather than
	// guessing how long that takes: a fixed sleep here would be flaky on a
	// slow/loaded machine (racing shortTimeoutSource.Sync below before the
	// child has the lock at all) and wastes time everywhere else.
	waitForFile(t, readyFile, 30*time.Second)

	shortTimeoutSource := sources.NewGitSourceWithLockTimeout(uri, sourceDir, runner, 10*time.Millisecond)
	did, err := shortTimeoutSource.Sync(u, true)
	assert.NoError(t, err)
	assert.False(t, did, "should have skipped syncing and fallen back to the existing copy")

	assert.NoError(t, cmd.Wait())
}
