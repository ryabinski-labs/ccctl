// Needs-input, resume, stop, and logging scenarios (REQ-008, REQ-009, REQ-010, REQ-014).
package tdd

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ryabinski-labs/claude-code-controller/internal/config"
	"github.com/ryabinski-labs/claude-code-controller/internal/session"
	"github.com/ryabinski-labs/claude-code-controller/internal/state"
	tu "github.com/ryabinski-labs/claude-code-controller/internal/testutil"
)

// SC-008-b
func TestSc008BHookCallFlipsThePaneToNeedsInput(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	launchN(h, 2)
	ws := h.Dial()
	h.WaitState(2, session.Running, 5*time.Second)
	cmd := exec.Command(h.CCCTL, "hook", "--slot", "2")
	cmd.Env = append(os.Environ(), "CCCTL_SOCK="+h.Sock)
	cmd.Stdin = strings.NewReader(`{"hook_event_name":"Notification","session_id":"` + h.Slot(2).SessionID + `"}`)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook: %v %s", err, b)
	}
	h.WaitState(2, session.NeedsInput, 2*time.Second)
	tu.Eventually(t, 2*time.Second, func() bool {
		for _, m := range ws.TextsCopy() {
			if info, ok := m["info"].(map[string]any); ok && m["type"] == "slot" && m["slot"] == 2.0 && info["state"] == "needs-input" {
				return true
			}
		}
		return false
	}, "no needs-input state frame")
	ev := h.Log.Events("needs_input")
	if len(ev) == 0 || ev[0]["hook"] != "Notification" || ev[0]["slot"] != 2.0 {
		t.Fatalf("needs_input events = %v", ev)
	}
}

// SC-008-c
func TestSc008CKeystrokeClearsNeedsInput(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	launchN(h, 2)
	h.WaitState(2, session.Running, 5*time.Second)
	h.App.M.Hook(2, "Stop", "")
	h.WaitState(2, session.NeedsInput, time.Second)
	ws := h.Dial()
	ws.Input(2, "y")
	h.WaitState(2, session.Running, 2*time.Second)
}

func writeState(t *testing.T, home string, slots []state.Slot) {
	if err := state.NewStore(config.Dir(home)).Save(slots); err != nil {
		t.Fatal(err)
	}
}

// SC-009-a
func TestSc009ALiveSessionsResumeInPlaceOthersDoNot(t *testing.T) {
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
	var cwds []string
	h := tu.Start(t, tu.Opts{ResumeGrace: 200 * time.Millisecond, Setup: func(home string) {
		var slots []state.Slot
		for i := 1; i <= 4; i++ {
			d := filepath.Join(home, "Documents", "w"+string(rune('0'+i)))
			os.MkdirAll(d, 0o755)
			cwds = append(cwds, d)
			st := "running"
			if i == 4 {
				st = "exited"
			}
			slots = append(slots, state.Slot{Slot: i, Task: "t" + string(rune('0'+i)), Cwd: d, SessionID: ids[i-1], State: st})
		}
		writeState(t, home, slots)
	}})
	tu.Eventually(t, 5*time.Second, func() bool { return len(h.Invocations()) == 3 }, "3 resumes not seen")
	bound := h.App.BoundAt()
	for i := 1; i <= 3; i++ {
		info := h.Slot(i)
		if info == nil || !info.LaunchedAt.Before(bound) {
			t.Fatalf("slot %d spawned %v, bound %v", i, info, bound)
		}
	}
	for _, inv := range h.Invocations() {
		var argv []string
		for _, a := range inv["argv"].([]any) {
			argv = append(argv, a.(string))
		}
		slot := int(inv["slot"].(string)[0] - '0')
		if slot == 4 {
			t.Fatal("slot 4 was relaunched")
		}
		if len(argv) != 6 || !reflect.DeepEqual(argv[1:5], []string{"--resume", ids[slot-1], "--dangerously-skip-permissions", "--settings"}) || !strings.HasSuffix(argv[5], ".json") {
			t.Errorf("slot %d argv = %q", slot, argv)
		}
		if resolved(inv["cwd"].(string)) != resolved(cwds[slot-1]) {
			t.Errorf("slot %d cwd = %v", slot, inv["cwd"])
		}
	}
	if h.Slot(4) != nil {
		t.Error("slot 4 shows a session")
	}
}

func failedResume(t *testing.T) (*tu.H, string, string) {
	id := uuid.NewString()
	var wt string
	h := tu.Start(t, tu.Opts{Mode: "resume-fail", Setup: func(home string) {
		wt = filepath.Join(home, "Documents", "folio-wt-t")
		os.MkdirAll(wt, 0o755)
		writeState(t, home, []state.Slot{{Slot: 1, Task: "t", Cwd: wt, Worktree: wt, Branch: "ctl/t", SessionID: id, State: "running"}})
	}})
	h.WaitState(1, session.ResumeFailed, 5*time.Second)
	return h, id, wt
}

// SC-009-b
func TestSc009BFailedResumeIsReported(t *testing.T) {
	h, _, _ := failedResume(t)
	if m := h.Slot(1).Message; m != "Could not resume t (exit code 1)." {
		t.Errorf("message = %q", m)
	}
	ev := h.Log.Events("resume_failed")
	if len(ev) == 0 || ev[0]["exit_code"] != 1.0 {
		t.Fatalf("resume_failed events = %v", ev)
	}
}

// SC-009-c
func TestSc009CStartFreshAfterAFailedResume(t *testing.T) {
	h, u, wt := failedResume(t)
	if code, b := h.Do("POST", "/api/sessions/1/fresh", nil); code != 201 {
		t.Fatalf("fresh = %d %s", code, b)
	}
	tu.Eventually(t, 5*time.Second, func() bool { return len(h.Invocations()) == 2 }, "no fresh invocation")
	inv := h.Invocations()[1]
	var argv []string
	for _, a := range inv["argv"].([]any) {
		argv = append(argv, a.(string))
	}
	i := slices.Index(argv, "--session-id")
	if i < 0 || argv[i+1] == u || slices.Contains(argv, "--resume") {
		t.Fatalf("argv = %q", argv)
	}
	v := argv[i+1]
	if resolved(inv["cwd"].(string)) != resolved(wt) || h.Slot(1).Slot != 1 {
		t.Errorf("cwd = %v", inv["cwd"])
	}
	f, _ := state.NewStore(config.Dir(h.Home)).Load()
	if len(f.Slots) != 1 || f.Slots[0].SessionID != v {
		t.Fatalf("state = %+v", f.Slots)
	}
}

// SC-010-a
func TestSc010AStopEscalatesFromSighupToSigkill(t *testing.T) {
	h := tu.Start(t, tu.Opts{Modes: map[int]string{2: "ignore-hup"}})
	repo := filepath.Join(h.Root, "folio")
	tu.GitRepo(t, repo)
	a := h.Launch("a", repo, true, "")
	b := h.Launch("b", repo, true, "")
	h.WaitState(1, session.Running, 5*time.Second)
	h.WaitState(2, session.Running, 5*time.Second)
	t0 := time.Now()
	h.App.M.Stop(1)
	h.App.M.Stop(2)
	h.WaitState(1, session.Exited, 2*time.Second)
	if c := *h.Slot(1).ExitCode; c != 129 {
		t.Errorf("slot 1 exit code = %d (want SIGHUP 129)", c)
	}
	h.WaitState(2, session.Exited, 7*time.Second)
	el := time.Since(t0)
	if c := *h.Slot(2).ExitCode; c != 137 {
		t.Errorf("slot 2 exit code = %d (want SIGKILL 137)", c)
	}
	if el < 5*time.Second || el > 6*time.Second {
		t.Errorf("slot 2 killed after %v", el)
	}
	for _, i := range []session.Info{a, b} {
		if _, err := os.Stat(i.Worktree); err != nil {
			t.Errorf("worktree %s gone", i.Worktree)
		}
		if !strings.Contains(branches(t, repo), i.Branch) {
			t.Errorf("branch %s gone", i.Branch)
		}
	}
	if m := h.Slot(1).Message; m != "Exited (code 129). Worktree kept at "+a.Worktree+" on branch ctl/a." {
		t.Errorf("message = %q", m)
	}
}

// SC-010-b
func TestSc010BSelfExitKeepsTheWorktree(t *testing.T) {
	h := tu.Start(t, tu.Opts{Mode: "exit=3"})
	repo := filepath.Join(h.Root, "folio")
	tu.GitRepo(t, repo)
	i := h.Launch("selfexit", repo, true, "")
	h.WaitState(1, session.Exited, 5*time.Second)
	if m := h.Slot(1).Message; m != "Exited (code 3). Worktree kept at "+i.Worktree+" on branch ctl/selfexit." {
		t.Errorf("message = %q", m)
	}
	if _, err := os.Stat(i.Worktree); err != nil {
		t.Error("worktree gone")
	}
}

// SC-014-a
func TestSc014ASessionLifecycleLogLines(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	d := filepath.Join(h.Root, "l")
	os.MkdirAll(d, 0o755)
	h.Launch("fix-login", d, false, "")
	h.WaitState(1, session.Running, 5*time.Second)
	h.App.M.Stop(1)
	h.WaitState(1, session.Exited, 3*time.Second)
	st := h.Log.Events("session_started")
	ex := h.Log.Events("session_exited")
	if len(st) != 1 || len(ex) != 1 {
		t.Fatalf("started=%d exited=%d", len(st), len(ex))
	}
	for _, k := range []string{"slot", "task", "cwd", "worktree", "branch", "session_id", "resumed"} {
		if _, ok := st[0][k]; !ok {
			t.Errorf("session_started lacks %s", k)
		}
	}
	if st[0]["resumed"] != false {
		t.Error("resumed != false")
	}
	for _, k := range []string{"slot", "task", "exit_code", "duration_s", "stopped_by_user"} {
		if _, ok := ex[0][k]; !ok {
			t.Errorf("session_exited lacks %s", k)
		}
	}
	if ex[0]["stopped_by_user"] != true {
		t.Error("stopped_by_user != true")
	}
}
