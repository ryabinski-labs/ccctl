// Green-phase scenarios from tdd/opencode-support.tdd.yaml (REQ-001, REQ-002,
// REQ-003, REQ-005, REQ-008, REQ-010, REQ-011).
package tdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ryabinski-labs/ccctl/internal/config"
	"github.com/ryabinski-labs/ccctl/internal/session"
	"github.com/ryabinski-labs/ccctl/internal/state"
	tu "github.com/ryabinski-labs/ccctl/internal/testutil"
)

func argvOf(inv map[string]any) []string {
	var argv []string
	for _, a := range inv["argv"].([]any) {
		argv = append(argv, a.(string))
	}
	return argv
}

func allInvocations(h *tu.H) []map[string]any {
	return append(h.Invocations(), h.OpenCodeInvocations()...)
}

func slotInvocations(h *tu.H, slot int) map[string]any {
	for _, inv := range allInvocations(h) {
		if inv["slot"] == string(rune('0'+slot)) {
			return inv
		}
	}
	return nil
}

// SC-001-b
func TestSc001BOpenCodeFreshArgv(t *testing.T) {
	got := session.OpenCodeFreshArgv("/opt/opencode", "U", "hello world")
	want := []string{"/opt/opencode", "--session", "U", "--auto", "--prompt", "hello world"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %q, want %q", got, want)
	}
	got = session.OpenCodeFreshArgv("/opt/opencode", "U", "")
	want = []string{"/opt/opencode", "--session", "U", "--auto"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv without prompt = %q, want %q", got, want)
	}
}

// SC-001-c
func TestSc001CClaudeFreshArgvUnchanged(t *testing.T) {
	got := session.FreshArgv("/h/.local/bin/claude", "U", "F", "hello")
	want := []string{"/h/.local/bin/claude", "--dangerously-skip-permissions", "--session-id", "U", "--settings", "F", "hello"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %q, want %q", got, want)
	}
	got = session.FreshArgv("/h/.local/bin/claude", "U", "F", "")
	want = []string{"/h/.local/bin/claude", "--dangerously-skip-permissions", "--session-id", "U", "--settings", "F"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv without prompt = %q, want %q", got, want)
	}
}

// SC-001-d
func TestSc001DUnknownAgentRefused(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	dir := filepath.Join(h.Root, "x")
	os.MkdirAll(dir, 0o755)
	code, body := h.Do("POST", "/api/sessions", map[string]any{"task": "t", "path": dir, "agent": "gemini"})
	var r struct{ Error string }
	json.Unmarshal(body, &r)
	if code != 400 || r.Error != session.MsgBadAgent {
		t.Fatalf("got %d %q", code, r.Error)
	}
	if n := len(allInvocations(h)); n != 0 {
		t.Fatalf("invocations = %d", n)
	}
	if h.Slot(1) != nil {
		t.Fatal("a session started")
	}
}

// SC-001-e
func TestSc001EMissingOpenCodeBinary(t *testing.T) {
	h := tu.Start(t, tu.Opts{OpenCodePath: "/nonexistent/opencode"})
	dir := filepath.Join(h.Root, "x")
	os.MkdirAll(dir, 0o755)
	code, body := h.Do("POST", "/api/sessions", map[string]any{"task": "t", "path": dir, "agent": "opencode"})
	var r struct{ Error string }
	json.Unmarshal(body, &r)
	if want := session.MsgOpenCodeNotFound("/nonexistent/opencode"); code != 400 || r.Error != want {
		t.Fatalf("got %d %q, want %q", code, r.Error, want)
	}
	if h.Slot(1) != nil {
		t.Fatal("a session started")
	}
}

// SC-001-f
func TestSc001FOpenCodeSessionID(t *testing.T) {
	id := session.NewSessionID(session.AgentOpenCode)
	if !strings.HasPrefix(id, session.OpenCodeSessionPrefix) {
		t.Fatalf("OpenCode ID = %q, want ses_ prefix", id)
	}
	if !session.ValidSessionID(session.AgentOpenCode, id) {
		t.Fatalf("OpenCode ID %q rejected by ValidSessionID", id)
	}
	if strings.Contains(id[len(session.OpenCodeSessionPrefix):], "-") {
		t.Fatalf("OpenCode ID = %q, want no UUID hyphens", id)
	}
	cid := session.NewSessionID(session.AgentClaude)
	if _, err := uuid.Parse(cid); err != nil {
		t.Fatalf("Claude ID = %q: %v", cid, err)
	}
	if session.ValidSessionID(session.AgentOpenCode, cid) {
		t.Fatalf("Claude UUID %q passed the OpenCode check", cid)
	}
}

// SC-002-b
func TestSc002BSeventhMixedLaunchRefused(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	agents := []session.Agent{session.AgentClaude, session.AgentOpenCode, session.AgentClaude, session.AgentOpenCode, session.AgentClaude, session.AgentOpenCode}
	for i, a := range agents {
		dir := filepath.Join(h.Root, "m"+string(rune('1'+i)))
		os.MkdirAll(dir, 0o755)
		h.LaunchAgent("mixed"+string(rune('1'+i)), dir, false, "", a)
	}
	tu.Eventually(t, 10*time.Second, func() bool { return len(allInvocations(h)) == 6 }, "six sessions did not start")
	extra := filepath.Join(h.Root, "extra")
	os.MkdirAll(extra, 0o755)
	code, body := h.Do("POST", "/api/sessions", map[string]any{"task": "extra", "path": extra, "agent": "claude"})
	var r struct{ Error string }
	json.Unmarshal(body, &r)
	if code != 409 || r.Error != session.MsgAllInUse {
		t.Fatalf("got %d %q", code, r.Error)
	}
	if n := len(allInvocations(h)); n != 6 {
		t.Fatalf("invocations = %d", n)
	}
}

// SC-002-c
func TestSc002CMixedSlotArgv(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	agents := []session.Agent{session.AgentClaude, session.AgentOpenCode, session.AgentClaude, session.AgentOpenCode, session.AgentClaude, session.AgentOpenCode}
	for i, a := range agents {
		dir := filepath.Join(h.Root, "s"+string(rune('1'+i)))
		os.MkdirAll(dir, 0o755)
		h.LaunchAgent("slot"+string(rune('1'+i)), dir, false, "", a)
	}
	tu.Eventually(t, 10*time.Second, func() bool { return len(allInvocations(h)) == 6 }, "six sessions did not start")
	for i, a := range agents {
		slot := i + 1
		inv := slotInvocations(h, slot)
		if inv == nil {
			t.Fatalf("slot %d has no invocation", slot)
		}
		argv := strings.Join(argvOf(inv), " ")
		info := h.Slot(slot)
		if info == nil || info.Agent != a {
			t.Fatalf("slot %d agent = %+v, want %s", slot, info, a)
		}
		if a == session.AgentOpenCode {
			if !strings.Contains(argv, "--session") || !strings.Contains(argv, "--auto") {
				t.Errorf("slot %d argv = %q", slot, argv)
			}
		} else if !strings.Contains(argv, "--dangerously-skip-permissions") {
			t.Errorf("slot %d argv = %q", slot, argv)
		}
	}
}

// SC-003-c
func TestSc003CEventForAnotherSessionIgnored(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	dir := filepath.Join(h.Root, "oc")
	os.MkdirAll(dir, 0o755)
	h.LaunchAgent("oc", dir, false, "", session.AgentOpenCode)
	h.WaitState(1, session.Running, 5*time.Second)
	if err := h.App.M.HookSource(1, "permission.asked", "ses_other", "opencode-plugin"); err != nil {
		t.Fatal(err)
	}
	if st := h.Slot(1).State; st != session.Running {
		t.Fatalf("state = %s", st)
	}
	if ev := h.Log.Events("needs_input"); len(ev) != 0 {
		t.Fatalf("needs_input events = %v", ev)
	}
}

// SC-003-d
func TestSc003DTypingClearsOpenCodeNeedsInput(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	dir := filepath.Join(h.Root, "oc")
	os.MkdirAll(dir, 0o755)
	h.LaunchAgent("oc", dir, false, "", session.AgentOpenCode)
	h.WaitState(1, session.Running, 5*time.Second)
	if err := h.App.M.HookSource(1, "permission.asked", h.Slot(1).SessionID, "opencode-plugin"); err != nil {
		t.Fatal(err)
	}
	h.WaitState(1, session.NeedsInput, time.Second)
	ws := h.Dial()
	ws.Input(1, "y")
	h.WaitState(1, session.Running, 2*time.Second)
	tu.Eventually(t, 2*time.Second, func() bool { return strings.Contains(string(h.OpenCodeInputLog(1)), "y") }, "input did not reach OpenCode")
}

// SC-005-a
func TestSc005AMixedSessionsResume(t *testing.T) {
	opencodeID := func() string { return session.OpenCodeSessionPrefix + strings.ReplaceAll(uuid.NewString(), "-", "") }
	ids := []string{uuid.NewString(), opencodeID(), uuid.NewString(), opencodeID(), uuid.NewString()}
	var cwds []string
	agents := []string{"claude", "opencode", "claude", "opencode", "claude"}
	h := tu.Start(t, tu.Opts{ResumeGrace: 200 * time.Millisecond, Setup: func(home string) {
		var slots []state.Slot
		for i := 0; i < 5; i++ {
			d := filepath.Join(home, "Documents", "r"+string(rune('1'+i)))
			os.MkdirAll(d, 0o755)
			cwds = append(cwds, d)
			st := "running"
			if i == 4 {
				st = "exited"
			}
			slots = append(slots, state.Slot{Slot: i + 1, Task: "resume" + string(rune('1'+i)), Agent: agents[i], Cwd: d, SessionID: ids[i], State: st})
		}
		if err := state.NewStore(config.Dir(home)).Save(slots); err != nil {
			t.Fatal(err)
		}
	}})
	tu.Eventually(t, 10*time.Second, func() bool { return len(allInvocations(h)) == 4 }, "four resumes not seen")
	bound := h.App.BoundAt()
	for i := 0; i < 4; i++ {
		info := h.Slot(i + 1)
		if info == nil || !info.LaunchedAt.Before(bound) {
			t.Fatalf("slot %d spawned %v, bound %v", i+1, info, bound)
		}
	}
	if h.Slot(5) != nil {
		t.Fatal("exited slot was relaunched")
	}
	for i := 0; i < 4; i++ {
		inv := slotInvocations(h, i+1)
		if inv == nil {
			t.Fatalf("slot %d has no invocation", i+1)
		}
		argv := argvOf(inv)
		if agents[i] == "opencode" {
			if !reflect.DeepEqual(argv, []string{"--session", ids[i], "--auto"}) && !reflect.DeepEqual(argv[1:], []string{"--session", ids[i], "--auto"}) {
				t.Errorf("slot %d argv = %q", i+1, argv)
			}
			for _, a := range argv {
				if a == "--prompt" {
					t.Errorf("slot %d resume passed a prompt: %q", i+1, argv)
				}
			}
		} else {
			if len(argv) < 6 || argv[1] != "--resume" || argv[2] != ids[i] || argv[3] != "--dangerously-skip-permissions" || argv[4] != "--settings" {
				t.Errorf("slot %d argv = %q", i+1, argv)
			}
		}
	}
}

// SC-005-b
func TestSc005BFailedOpenCodeResume(t *testing.T) {
	id := session.OpenCodeSessionPrefix + strings.ReplaceAll(uuid.NewString(), "-", "")
	h := tu.Start(t, tu.Opts{ResumeGrace: 2 * time.Second, Modes: map[int]string{2: "resume-fail"}, Setup: func(home string) {
		d := filepath.Join(home, "Documents", "rf")
		os.MkdirAll(d, 0o755)
		if err := state.NewStore(config.Dir(home)).Save([]state.Slot{{Slot: 2, Task: "opencode-task", Agent: "opencode", Cwd: d, SessionID: id, State: "running"}}); err != nil {
			t.Fatal(err)
		}
	}})
	h.WaitState(2, session.ResumeFailed, 5*time.Second)
	if got, want := h.Slot(2).Message, session.MsgResumeFailed("opencode-task", 1); got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	ev := h.Log.Events("resume_failed")
	if len(ev) != 1 || ev[0]["agent"] != "opencode" {
		t.Fatalf("resume_failed = %v", ev)
	}
}

// SC-005-c
func TestSc005CStartFreshKeepsOpenCode(t *testing.T) {
	id := session.OpenCodeSessionPrefix + strings.ReplaceAll(uuid.NewString(), "-", "")
	cwd := ""
	h := tu.Start(t, tu.Opts{ResumeGrace: 2 * time.Second, Modes: map[int]string{2: "resume-fail"}, Setup: func(home string) {
		cwd = filepath.Join(home, "Documents", "rf")
		os.MkdirAll(cwd, 0o755)
		if err := state.NewStore(config.Dir(home)).Save([]state.Slot{{Slot: 2, Task: "opencode-task", Agent: "opencode", Cwd: cwd, Worktree: cwd, Branch: "ctl/opencode-task", SessionID: id, State: "running"}}); err != nil {
			t.Fatal(err)
		}
	}})
	h.WaitState(2, session.ResumeFailed, 5*time.Second)
	old := h.Slot(2)
	code, body := h.Do("POST", "/api/sessions/2/fresh", nil)
	if code != 201 {
		t.Fatalf("fresh = %d %s", code, body)
	}
	var r struct{ Info session.Info }
	json.Unmarshal(body, &r)
	if r.Info.Agent != session.AgentOpenCode || r.Info.SessionID == old.SessionID || r.Info.Cwd != cwd {
		t.Fatalf("fresh info = %+v", r.Info)
	}
	tu.Eventually(t, 5*time.Second, func() bool { return len(h.OpenCodeInvocations()) == 2 }, "fresh OpenCode did not start")
	inv := h.OpenCodeInvocations()[1]
	argv := strings.Join(argvOf(inv), " ")
	if !strings.Contains(argv, "--session "+r.Info.SessionID) || !strings.Contains(argv, "--auto") {
		t.Fatalf("fresh argv = %q", argv)
	}
	if r.Info.Worktree != old.Worktree || r.Info.Branch != old.Branch {
		t.Fatalf("worktree/branch changed: %+v -> %+v", old, r.Info)
	}
}

// SC-005-d
func TestSc005DLegacyOpenCodeSessionIDRepaired(t *testing.T) {
	legacy := uuid.NewString()
	var cwd string
	h := tu.Start(t, tu.Opts{Setup: func(home string) {
		cwd = filepath.Join(home, "Documents", "legacy")
		os.MkdirAll(cwd, 0o755)
		if err := state.NewStore(config.Dir(home)).Save([]state.Slot{{Slot: 1, Task: "legacy", Agent: "opencode", Cwd: cwd, SessionID: legacy, State: "running"}}); err != nil {
			t.Fatal(err)
		}
	}})
	tu.Eventually(t, 10*time.Second, func() bool { return len(h.OpenCodeInvocations()) == 1 }, "legacy OpenCode slot did not start")
	inv := h.OpenCodeInvocations()[0]
	argv := strings.Join(argvOf(inv), " ")
	if strings.Contains(argv, legacy) {
		t.Fatalf("legacy UUID was passed to OpenCode: %q", argv)
	}
	if !strings.Contains(argv, "--session "+session.OpenCodeSessionPrefix) {
		t.Fatalf("argv = %q", argv)
	}
	if got := h.Slot(1).SessionID; !strings.HasPrefix(got, session.OpenCodeSessionPrefix) {
		t.Fatalf("slot session ID = %q", got)
	}
	if ev := h.Log.Events("resume_bad_session_id"); len(ev) != 1 {
		t.Fatalf("resume_bad_session_id events = %v", ev)
	}
}

// SC-008-a
func TestSc008AOpenCodeLaunchLatency(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	var max time.Duration
	for i := 1; i <= 5; i++ {
		dir := filepath.Join(h.Root, "lat"+string(rune('0'+i)))
		os.MkdirAll(dir, 0o755)
		start := time.Now()
		h.LaunchAgent("lat"+string(rune('0'+i)), dir, false, "", session.AgentOpenCode)
		h.WaitState(i, session.Running, 5*time.Second)
		if d := time.Since(start); d > max {
			max = d
		}
	}
	if max > 5*time.Second {
		t.Fatalf("max launch latency = %s", max)
	}
}

// SC-008-b
func TestSc008BNeedsInputLatency(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	dir := filepath.Join(h.Root, "lat")
	os.MkdirAll(dir, 0o755)
	h.LaunchAgent("lat", dir, false, "", session.AgentOpenCode)
	h.WaitState(1, session.Running, 5*time.Second)
	ws := h.Dial()
	var max time.Duration
	for i := 0; i < 5; i++ {
		start := time.Now()
		if err := h.App.M.HookSource(1, "permission.asked", h.Slot(1).SessionID, "opencode-plugin"); err != nil {
			t.Fatal(err)
		}
		h.WaitState(1, session.NeedsInput, 2*time.Second)
		if d := time.Since(start); d > max {
			max = d
		}
		ws.Input(1, "y")
		h.WaitState(1, session.Running, 2*time.Second)
	}
	if max > 2*time.Second {
		t.Fatalf("max needs-input latency = %s", max)
	}
}

// SC-010-a
func TestSc010AMissingAgentDefaultsToClaude(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	dir := filepath.Join(h.Root, "default")
	os.MkdirAll(dir, 0o755)
	code, body := h.Do("POST", "/api/sessions", map[string]any{"task": "default", "path": dir})
	if code != 201 {
		t.Fatalf("launch = %d %s", code, body)
	}
	tu.Eventually(t, 5*time.Second, func() bool { return len(h.Invocations()) == 1 }, "claude did not start")
	if argv := strings.Join(argvOf(h.Invocations()[0]), " "); !strings.Contains(argv, "--dangerously-skip-permissions") {
		t.Fatalf("argv = %q", argv)
	}
	f, err := state.NewStore(config.Dir(h.Home)).Load()
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != state.Version || len(f.Slots) != 1 || f.Slots[0].Agent != "claude" {
		t.Fatalf("state = %+v", f)
	}
}

// SC-010-b
func TestSc010BStateV1LoadsAndV2Saves(t *testing.T) {
	st := state.NewStore(t.TempDir())
	if err := os.WriteFile(st.Path, []byte(`{"version":1,"slots":[{"slot":1,"task":"old"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if f.Slots[0].Agent != "claude" {
		t.Fatalf("v1 agent = %q", f.Slots[0].Agent)
	}
	if err := st.Save([]state.Slot{{Slot: 1, Task: "new", Agent: "opencode"}}); err != nil {
		t.Fatal(err)
	}
	f, err = st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != 2 || f.Slots[0].Agent != "opencode" {
		t.Fatalf("saved = %+v", f)
	}
}

// SC-010-c
func TestSc010CStateContractIncludesAgent(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	for i, a := range []session.Agent{session.AgentClaude, session.AgentOpenCode} {
		dir := filepath.Join(h.Root, "c"+string(rune('1'+i)))
		os.MkdirAll(dir, 0o755)
		h.LaunchAgent("contract"+string(rune('1'+i)), dir, false, "", a)
	}
	tu.Eventually(t, 10*time.Second, func() bool { return len(allInvocations(h)) == 2 }, "sessions did not start")
	code, body := h.Do("GET", "/api/state", nil)
	if code != 200 {
		t.Fatalf("state = %d %s", code, body)
	}
	var got struct {
		Slots []map[string]any `json:"slots"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Slots) != config.MaxSessions || got.Slots[0]["agent"] != "claude" || got.Slots[1]["agent"] != "opencode" {
		t.Fatalf("slots = %v", got.Slots)
	}
	for _, k := range []string{"slot", "task", "cwd", "repo", "worktree", "branch", "session_id", "state", "launched_at", "resumed", "exit_code", "message", "agent"} {
		if _, ok := got.Slots[0][k]; !ok {
			t.Errorf("slot is missing %q", k)
		}
	}
	ws := h.Dial()
	tu.Eventually(t, 2*time.Second, func() bool {
		for _, m := range ws.TextsCopy() {
			if m["type"] == "hello" {
				return true
			}
		}
		return false
	}, "no hello frame")
}

// SC-011-a
func TestSc011ALogsCarryAgent(t *testing.T) {
	const canary = "sk-test-CANARY"
	h := tu.Start(t, tu.Opts{Setup: func(home string) {
		os.MkdirAll(config.Dir(home), 0o700)
		os.WriteFile(filepath.Join(config.Dir(home), "config.toml"), []byte("[env]\nCANARY_TOKEN = \""+canary+"\"\n"), 0o600)
	}})
	for i, a := range []session.Agent{session.AgentClaude, session.AgentOpenCode} {
		dir := filepath.Join(h.Root, "log"+string(rune('1'+i)))
		os.MkdirAll(dir, 0o755)
		h.LaunchAgent("log"+string(rune('1'+i)), dir, false, "", a)
	}
	tu.Eventually(t, 10*time.Second, func() bool { return len(allInvocations(h)) == 2 }, "sessions did not start")
	h.WaitState(2, session.Running, 5*time.Second)
	if err := h.App.M.HookSource(2, "permission.asked", h.Slot(2).SessionID, "opencode-plugin"); err != nil {
		t.Fatal(err)
	}
	h.WaitState(2, session.NeedsInput, time.Second)
	h.Do("POST", "/api/sessions/1/stop", nil)
	h.Do("POST", "/api/sessions/2/stop", nil)
	tu.Eventually(t, 5*time.Second, func() bool {
		return h.Slot(1).State == session.Exited && h.Slot(2).State == session.Exited
	}, "sessions did not exit")

	started := h.Log.Events("session_started")
	if len(started) != 2 {
		t.Fatalf("session_started = %v", started)
	}
	agents := map[any]bool{started[0]["agent"]: true, started[1]["agent"]: true}
	if !agents["claude"] || !agents["opencode"] {
		t.Fatalf("started agents = %v", agents)
	}
	need := h.Log.Events("needs_input")
	if len(need) != 1 || need[0]["agent"] != "opencode" || need[0]["source"] != "opencode-plugin" {
		t.Fatalf("needs_input = %v", need)
	}
	exited := h.Log.Events("session_exited")
	if len(exited) != 2 {
		t.Fatalf("session_exited = %v", exited)
	}
	if strings.Contains(h.Log.String(), canary) {
		t.Fatal("credential value reached the log")
	}
}
