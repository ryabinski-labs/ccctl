// Launch, slot, and worktree scenarios (REQ-001, REQ-002, REQ-007).
package tdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ryabinski-labs/claude-code-controller/internal/session"
	tu "github.com/ryabinski-labs/claude-code-controller/internal/testutil"
)

func resolved(p string) string { r, _ := filepath.EvalSymlinks(p); return r }

// SC-001-b
func TestSc001BWorktreeIsASiblingFolderOnCtlTaskFromHead(t *testing.T) {
	root := resolved(t.TempDir())
	repo := filepath.Join(root, "folio")
	head := tu.GitRepo(t, repo)
	plan, err := session.PlanWorktree(repo, "fix-login")
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Check("fix-login"); err != nil {
		t.Fatal(err)
	}
	if err := plan.Create(); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, "folio-wt-fix-login")
	if b, _ := tu.Git(wt, "rev-parse", "--abbrev-ref", "HEAD"); b != "ctl/fix-login" {
		t.Errorf("branch = %q", b)
	}
	if c, _ := tu.Git(wt, "rev-parse", "HEAD"); c != head {
		t.Errorf("HEAD = %q; want %q", c, head)
	}
}

func branches(t *testing.T, repo string) string { b, _ := tu.Git(repo, "branch", "--list"); return b }

func folders(dir string) string {
	es, _ := os.ReadDir(dir)
	var n []string
	for _, e := range es {
		n = append(n, e.Name())
	}
	return strings.Join(n, ",")
}

// SC-002-c
func TestSc002CExistingWorktreeFolderOrBranchBlocksLaunch(t *testing.T) {
	const msg = "Worktree folder or branch for fix-login already exists. Choose another task name."
	for _, variant := range []string{"folder", "branch"} {
		t.Run(variant, func(t *testing.T) {
			h := tu.Start(t, tu.Opts{})
			repo := filepath.Join(h.Root, "folio")
			tu.GitRepo(t, repo)
			if variant == "folder" {
				os.MkdirAll(filepath.Join(h.Root, "folio-wt-fix-login"), 0o755)
			} else {
				tu.Git(repo, "branch", "ctl/fix-login")
			}
			bb, fb := branches(t, repo), folders(h.Root)
			code, body := h.Do("POST", "/api/sessions", session.LaunchRequest{Task: "fix-login", Path: repo, Worktree: true})
			var r struct{ Error string }
			json.Unmarshal(body, &r)
			if code != 400 || r.Error != msg {
				t.Fatalf("got %d %q", code, r.Error)
			}
			if n := len(h.Invocations()); n != 0 {
				t.Errorf("claude invoked %d times", n)
			}
			if branches(t, repo) != bb || folders(h.Root) != fb {
				t.Error("branches or folders changed")
			}
		})
	}
}

// SC-002-d
func TestSc002DNonGitFolderRunsWithoutAWorktree(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	notes := filepath.Join(h.Root, "notes")
	os.MkdirAll(notes, 0o755)
	code, body := h.Do("GET", "/api/inspect?path="+notes, nil)
	var in struct {
		WorktreeAllowed bool `json:"worktree_allowed"`
		Note            string
	}
	json.Unmarshal(body, &in)
	if code != 200 || in.WorktreeAllowed || in.Note != "Not a git repository: session runs in the folder itself." {
		t.Fatalf("inspect = %d %s", code, body)
	}
	h.Launch("notes-task", notes, false, "")
	ws := h.Dial()
	ws.WaitOutput(1, "FAKE_CLAUDE cwd="+notes+" ", 5*time.Second)
	if m, _ := filepath.Glob(filepath.Join(h.Root, "notes-wt-*")); len(m) != 0 {
		t.Errorf("worktree created: %v", m)
	}
}

// SC-002-e
func TestSc002EMissingClaudeBinaryIsReported(t *testing.T) {
	h := tu.Start(t, tu.Opts{ClaudePath: "/nonexistent/claude"})
	dir := filepath.Join(h.Root, "x")
	os.MkdirAll(dir, 0o755)
	code, body := h.Do("POST", "/api/sessions", session.LaunchRequest{Task: "t", Path: dir})
	var r struct{ Error string }
	json.Unmarshal(body, &r)
	if code != 400 || r.Error != "Claude Code was not found at /nonexistent/claude. Set claude_path in ~/.ccctl/config.toml." {
		t.Fatalf("got %d %q", code, r.Error)
	}
	if h.Slot(1) != nil {
		t.Error("a session started")
	}
}

// launchN launches n sessions in fresh plain folders and returns their folders.
func launchN(h *tu.H, n int) []string {
	var dirs []string
	for i := 1; i <= n; i++ {
		d := filepath.Join(h.Root, "f"+string(rune('0'+i)))
		os.MkdirAll(d, 0o755)
		h.Launch("task"+string(rune('0'+i)), d, false, "")
		dirs = append(dirs, d)
	}
	return dirs
}

// SC-007-b
func TestSc007BFifthActiveLaunchIsRefused(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	launchN(h, 4)
	tu.Eventually(t, 5*time.Second, func() bool { return len(h.Invocations()) == 4 }, "4 sessions did not start")
	d := filepath.Join(h.Root, "five")
	os.MkdirAll(d, 0o755)
	code, body := h.Do("POST", "/api/sessions", session.LaunchRequest{Task: "five", Path: d})
	if code != 409 || !strings.Contains(string(body), "All 4 slots are in use. Stop or close a session first.") {
		t.Fatalf("got %d %s", code, body)
	}
	time.Sleep(300 * time.Millisecond)
	if n := len(h.Invocations()); n != 4 {
		t.Fatalf("invocations = %d", n)
	}
}

// SC-007-c
func TestSc007CExitedPaneIsReplacedAndItsWorktreeKept(t *testing.T) {
	h := tu.Start(t, tu.Opts{Modes: map[int]string{4: "exit=0"}})
	launchN(h, 3)
	repo := filepath.Join(h.Root, "folio")
	tu.GitRepo(t, repo)
	old := h.Launch("old-task", repo, true, "")
	if old.Slot != 4 {
		t.Fatalf("old-task slot = %d", old.Slot)
	}
	h.WaitState(4, session.Exited, 5*time.Second)
	nw := h.Launch("new-task", filepath.Join(h.Root, "f1"), false, "")
	if nw.Slot != 4 || h.Slot(4).Task != "new-task" {
		t.Fatalf("new-task slot = %d", nw.Slot)
	}
	if _, err := os.Stat(old.Worktree); err != nil {
		t.Errorf("worktree gone: %v", err)
	}
	if !strings.Contains(branches(t, repo), "ctl/old-task") {
		t.Error("branch ctl/old-task gone")
	}
}

// SC-001-d
func TestSc001DConfigEnvReachesEverySessionAndReloads(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	write := func(v string) {
		os.WriteFile(filepath.Join(h.Home, ".ccctl", "config.toml"), []byte("[env]\nGEMINI_API_KEY = \""+v+"\"\nCCCTL_SOCK = \"/evil\"\n"), 0o600)
	}
	envOf := func(i int) []string {
		var out []string
		for _, e := range h.Invocations()[i]["env"].([]any) {
			out = append(out, e.(string))
		}
		return out
	}
	write("key-one")
	d1, d2 := filepath.Join(h.Root, "e1"), filepath.Join(h.Root, "e2")
	os.MkdirAll(d1, 0o755)
	os.MkdirAll(d2, 0o755)
	h.Launch("env-one", d1, false, "")
	tu.Eventually(t, 5*time.Second, func() bool { return len(h.Invocations()) == 1 }, "not started")
	e := envOf(0)
	if !slices.Contains(e, "GEMINI_API_KEY=key-one") || slices.Contains(e, "CCCTL_SOCK=/evil") {
		t.Fatalf("session 1 env lacks key or allows CCCTL_ override")
	}
	write("key-two") // edited while running: applies to the next launch without a restart
	h.Launch("env-two", d2, false, "")
	tu.Eventually(t, 5*time.Second, func() bool { return len(h.Invocations()) == 2 }, "not started")
	if !slices.Contains(envOf(1), "GEMINI_API_KEY=key-two") {
		t.Fatal("session 2 did not get the reloaded key")
	}
	if strings.Contains(h.Log.String(), "key-one") || strings.Contains(h.Log.String(), "key-two") {
		t.Fatal("a key value reached the log")
	}
}
