// Unit-level scenarios from tdd/claude-code-controller.tdd.yaml. Test names keep the scenario IDs.
package tdd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ryabinski-labs/claude-code-controller/internal/app"
	"github.com/ryabinski-labs/claude-code-controller/internal/auth"
	"github.com/ryabinski-labs/claude-code-controller/internal/config"
	"github.com/ryabinski-labs/claude-code-controller/internal/repos"
	"github.com/ryabinski-labs/claude-code-controller/internal/session"
	"github.com/ryabinski-labs/claude-code-controller/internal/state"
	"github.com/ryabinski-labs/claude-code-controller/internal/tailscale"
)

// SC-001-c
func TestSc001CFreshLaunchArgv(t *testing.T) {
	c, u, f := "/h/.local/bin/claude", "U", "F"
	want := []string{c, "--dangerously-skip-permissions", "--session-id", u, "--settings", f, "hello"}
	if got := session.FreshArgv(c, u, f, "hello"); !reflect.DeepEqual(got, want) {
		t.Fatalf("with prompt: %q", got)
	}
	if got := session.FreshArgv(c, u, f, ""); !reflect.DeepEqual(got, want[:6]) {
		t.Fatalf("without prompt: %q", got)
	}
}

// SC-002-a
func TestSc002ATaskNameRulesAndBoundaries(t *testing.T) {
	open := []string{"fix-login"}
	for in, want := range map[string]string{"a": "a", strings.Repeat("a", 40): strings.Repeat("a", 40), "Fix-Api": "fix-api"} {
		got, err := session.ValidateTask(in, open)
		if err != nil || got != want {
			t.Errorf("ValidateTask(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", strings.Repeat("a", 41), "fix_login", "fix login", "café", "fix-login"} {
		_, err := session.ValidateTask(in, open)
		if err == nil || err.Error() != "Task name must be 1 to 40 characters of a-z, 0-9 or -, and unique among open sessions." {
			t.Errorf("ValidateTask(%q) err = %v", in, err)
		}
	}
}

// SC-002-b
func TestSc002BCustomPathRules(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "f.txt")
	os.WriteFile(f, []byte("x"), 0o644)
	if _, err := session.ValidatePath("/opt/x"); err == nil || err.Error() != "Path /opt/x does not exist or is not a folder." {
		t.Errorf("/opt/x: %v", err)
	}
	if _, err := session.ValidatePath(f); err == nil || err.Error() != "Path "+f+" does not exist or is not a folder." {
		t.Errorf("file: %v", err)
	}
	if _, err := session.ValidatePath("rel/path"); err == nil {
		t.Error("relative path accepted")
	}
	if got, err := session.ValidatePath(d); err != nil || got != d {
		t.Errorf("dir: %q %v", got, err)
	}
}

// SC-002-f
func TestSc002FFirstPromptLengthBoundary(t *testing.T) {
	if err := session.ValidatePrompt(strings.Repeat("x", 10000)); err != nil {
		t.Errorf("10000 refused: %v", err)
	}
	if err := session.ValidatePrompt(strings.Repeat("x", 10001)); err == nil {
		t.Error("10001 accepted")
	}
}

// SC-003-a
func TestSc003ARepoScanDepthAndMissingRoots(t *testing.T) {
	home := t.TempDir()
	for _, p := range []string{"Documents/a", "Documents/x/y/b", "Documents/x/y/z/c", "d"} {
		os.MkdirAll(filepath.Join(home, p, ".git"), 0o755)
	}
	var c config.Config
	c.ApplyDefaults(home, func(string) (string, error) { return "", errors.New("none") })
	got := map[string]bool{}
	for _, r := range repos.Scan(home, c.Roots) {
		got[r.Path] = true
	}
	want := map[string]bool{filepath.Join(home, "Documents/a"): true, filepath.Join(home, "Documents/x/y/b"): true, filepath.Join(home, "d"): true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scan = %v; want %v", got, want)
	}
}

// SC-005-a
func TestSc005AAuthorizationDecisionTable(t *testing.T) {
	allow := []string{"owner@example"}
	for login, want := range map[string]bool{"owner@example": true, "other@example": false, "": false} {
		if got := auth.Authorize(allow, login); got != want {
			t.Errorf("Authorize(%q) = %v", login, got)
		}
	}
}

// SC-005-f
func TestSc005FDefaultAllowlistIsTheHostNodeSLogin(t *testing.T) {
	fixture := `{"BackendState":"Running","Self":{"UserID":42,"HostName":"mac-mini","DNSName":"mac-mini.tail.ts.net.","TailscaleIPs":["100.64.0.10","fd7a::1"]},"User":{"42":{"LoginName":"owner@example"}}}`
	st, err := tailscale.ParseStatus([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if st.IPv4 != "100.64.0.10" {
		t.Errorf("IPv4 = %q", st.IPv4)
	}
	home := t.TempDir()
	c, err := config.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if got := app.EffectiveAllowlist(c, st); !reflect.DeepEqual(got, []string{"owner@example"}) {
		t.Fatalf("allowlist = %v", got)
	}
}

// SC-005-g
func TestSc005GWhoisResultsCached60SecondsPerAddress(t *testing.T) {
	now := time.Unix(0, 0)
	calls := 0
	c := auth.NewCache(func(context.Context, string) (string, error) { calls++; return "owner@example", nil })
	c.Now = func() time.Time { return now }
	for _, at := range []time.Duration{0, 59 * time.Second} {
		now = time.Unix(0, 0).Add(at)
		c.Login(context.Background(), "100.64.0.20:51000")
	}
	if calls != 1 {
		t.Fatalf("calls after t=59s = %d", calls)
	}
	now = time.Unix(61, 0)
	c.Login(context.Background(), "100.64.0.20:51001")
	if calls != 2 {
		t.Fatalf("calls after t=61s = %d", calls)
	}
}

// SC-006-a
func TestSc006ATailscaleCliDiscoveryOrder(t *testing.T) {
	onPath := func(string) (string, error) { return "/usr/bin/tailscale", nil }
	notOnPath := func(string) (string, error) { return "", errors.New("no") }
	yes := func(string) bool { return true }
	no := func(string) bool { return false }
	check := func(name, got, want string, err error) {
		if err != nil || got != want {
			t.Errorf("%s: %q %v", name, got, err)
		}
	}
	p, err := tailscale.ResolveCLI("/cfg/tailscale", onPath, yes)
	check("config", p, "/cfg/tailscale", err)
	p, err = tailscale.ResolveCLI("", onPath, yes)
	check("PATH", p, "/usr/bin/tailscale", err)
	p, err = tailscale.ResolveCLI("", notOnPath, yes)
	check("bundle", p, "/Applications/Tailscale.app/Contents/MacOS/Tailscale", err)
	_, err = tailscale.ResolveCLI("", notOnPath, no)
	if err == nil || err.Error() != "Tailscale CLI not found. Install Tailscale or set tailscale_path in ~/.ccctl/config.toml." {
		t.Errorf("none: %v", err)
	}
}

// SC-006-b
func TestSc006BMonitorIntervalAndTransitions(t *testing.T) {
	m := tailscale.NewMonitor()
	if m.Interval != 10*time.Second {
		t.Errorf("interval = %v", m.Interval)
	}
	var got []tailscale.Action
	for _, s := range []string{"Running", "Stopped", "NeedsLogin", "Running"} {
		got = append(got, m.Observe(s))
	}
	if want := []tailscale.Action{tailscale.Bind, tailscale.Unbind, tailscale.NoOp, tailscale.Bind}; !reflect.DeepEqual(got, want) {
		t.Fatalf("actions = %v", got)
	}
}

// SC-007-a
func TestSc007ASlotAllocationTable(t *testing.T) {
	const e, r, s, n, x, f = session.State(""), session.Running, session.Starting, session.NeedsInput, session.Exited, session.ResumeFailed
	for _, c := range []struct {
		in   []session.State
		want int
	}{{[]session.State{e, r, r, r}, 1}, {[]session.State{r, r, r, x}, 4}, {[]session.State{r, f, x, r}, 2}, {[]session.State{r, s, n, r}, 0}} {
		if got := session.NextSlot(c.in); got != c.want {
			t.Errorf("NextSlot(%v) = %d; want %d", c.in, got, c.want)
		}
	}
}

// SC-008-a
func TestSc008AGeneratedHookSettings(t *testing.T) {
	b, err := session.HookSettings("C", 2)
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Hooks map[string][]struct {
			Hooks []struct{ Type, Command string }
		}
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	for _, ev := range []string{"Notification", "Stop"} {
		if got := s.Hooks[ev][0].Hooks[0].Command; got != "C hook --slot 2" {
			t.Errorf("%s command = %q", ev, got)
		}
	}
}

// SC-009-d
func TestSc009DStateFileDurabilityAndVersionGuard(t *testing.T) {
	st := state.NewStore(t.TempDir())
	if err := st.Save([]state.Slot{{Slot: 1, Task: "old"}}); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(st.Path)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
	f, err := st.Load()
	if err != nil || f.Version != 1 || f.Slots[0].Task != "old" {
		t.Fatalf("load = %+v %v", f, err)
	}
	before, _ := os.ReadFile(st.Path)
	st.BeforeRename = func() error { return errors.New("crash") }
	if err := st.Save([]state.Slot{{Slot: 1, Task: "new"}}); err == nil {
		t.Fatal("interrupted save reported success")
	}
	after, _ := os.ReadFile(st.Path)
	if string(before) != string(after) {
		t.Fatal("interrupted save changed the file")
	}
	os.WriteFile(st.Path, []byte(`{"version":2,"slots":[]}`), 0o600)
	if _, err := st.Load(); err == nil || err.Error() != "State file version 2 is newer than this ccctl supports." {
		t.Fatalf("version 2: %v", err)
	}
}

// SC-011-a
func TestSc011ARingBufferKeepsTheNewest1Mib(t *testing.T) {
	r := session.NewRing(session.RingSize)
	in := make([]byte, 1048586)
	for i := range in {
		in[i] = byte(i * 7)
	}
	// Write in uneven chunks to exercise wraparound.
	for off := 0; off < len(in); {
		n := min(4093, len(in)-off)
		r.Write(in[off : off+n])
		off += n
	}
	snap := r.Snapshot()
	if len(snap) != 1048576 || string(snap) != string(in[10:]) {
		t.Fatalf("len=%d match=%v", len(snap), string(snap) == string(in[10:]))
	}
}

// SC-006-h
func TestSc006HTailscaleCliRunsInCliModeUnderAServiceManager(t *testing.T) {
	env := tailscale.CLIEnv([]string{"HOME=/h", "PATH=/usr/bin:/bin"})
	if !slices.Contains(env, "TAILSCALE_BE_CLI=1") || !slices.Contains(env, "HOME=/h") {
		t.Fatalf("env = %v", env)
	}
}

// SC-003-b
func TestSc003BRepoScanSkipsLinkedWorktrees(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "Documents/folio/.git"), 0o755)
	os.MkdirAll(filepath.Join(home, "Documents/folio-wt-fix-login"), 0o755)
	os.WriteFile(filepath.Join(home, "Documents/folio-wt-fix-login/.git"), []byte("gitdir: ../folio/.git/worktrees/fix-login\n"), 0o644)
	got := repos.Scan(home, []string{filepath.Join(home, "Documents")})
	if len(got) != 1 || got[0].Name != "folio" {
		t.Fatalf("scan = %+v", got)
	}
}
