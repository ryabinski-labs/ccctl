// Resume by ID: start `claude --resume <id>` for a session saved under ~/.claude/projects.
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

	"github.com/ryabinski-labs/ccctl/internal/session"
	tu "github.com/ryabinski-labs/ccctl/internal/testutil"
)

// writeTranscript saves a minimal claude transcript for id whose session ran in cwd.
func writeTranscript(t *testing.T, home, id, cwd string, extra ...string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", strings.ReplaceAll(cwd, "/", "-"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := append([]string{
		`{"type":"mode","sessionId":"` + id + `"}`,
		`{"type":"user","cwd":"` + cwd + `","gitBranch":"main","sessionId":"` + id + `"}`,
		`{"type":"ai-title","aiTitle":"Quota issue","sessionId":"` + id + `"}`,
	}, extra...)
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResumeByIDParsesPastedInput(t *testing.T) {
	id := "18cf1881-5565-4e54-b0fd-ffafef9e86b1"
	for _, in := range []string{id, "  " + strings.ToUpper(id) + "\n", "claude --resume " + id, "claude -r " + id + " --dangerously-skip-permissions"} {
		if got, err := session.ParseSessionID(in); err != nil || got != id {
			t.Errorf("ParseSessionID(%q) = %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "claude --resume", "18cf1881", id + " " + uuid.NewString()} {
		if _, err := session.ParseSessionID(in); err == nil {
			t.Errorf("ParseSessionID(%q) accepted", in)
		}
	}
}

func TestResumeByIDReadsOnlyTheEndsOfALargeTranscript(t *testing.T) {
	home, id, cwd := t.TempDir(), uuid.NewString(), "/somewhere/repo"
	pad := `{"type":"assistant","message":"` + strings.Repeat("x", 4000) + `"}`
	var mid []string
	for range 1000 { // ~4 MB, larger than head + tail
		mid = append(mid, pad)
	}
	mid = append(mid, `{"type":"ai-title","aiTitle":"Renamed later","sessionId":"`+id+`"}`)
	writeTranscript(t, home, id, cwd, mid...)
	tr, err := session.FindTranscript(filepath.Join(home, ".claude"), id)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Cwd != cwd || tr.Title != "Renamed later" || tr.Branch != "main" {
		t.Fatalf("transcript = %+v", tr)
	}
	if _, err := session.FindTranscript(filepath.Join(home, ".claude"), uuid.NewString()); err == nil {
		t.Fatal("unknown id found")
	}
}

func TestResumeByIDLaunchesClaudeResumeInTheRecordedFolder(t *testing.T) {
	id := uuid.NewString()
	var cwd string
	h := tu.Start(t, tu.Opts{ResumeGrace: 200 * time.Millisecond, Setup: func(home string) {
		cwd = filepath.Join(home, "Documents", "waf")
		os.MkdirAll(cwd, 0o755)
		writeTranscript(t, home, id, cwd)
	}})

	code, body := h.Do("GET", "/api/transcript?id="+"claude%20--resume%20"+id, nil)
	var tr session.Transcript
	if code != 200 || json.Unmarshal(body, &tr) != nil || tr.Cwd != cwd || tr.Title != "Quota issue" || tr.OpenSlot != 0 {
		t.Fatalf("lookup = %d %s", code, body)
	}

	if code, body := h.Do("POST", "/api/sessions", map[string]any{"task": "quota", "resume_id": id, "prompt": "hi"}); code != 400 ||
		!strings.Contains(string(body), "cannot take a first prompt") {
		t.Fatalf("resume with prompt = %d %s", code, body)
	}

	code, body = h.Do("POST", "/api/sessions", map[string]any{"task": "quota", "resume_id": "claude --resume " + id, "slot": 2})
	if code != 201 {
		t.Fatalf("launch = %d %s", code, body)
	}
	tu.Eventually(t, 5*time.Second, func() bool { return len(h.Invocations()) == 1 }, "claude never started")
	inv := h.Invocations()[0]
	var argv []string
	for _, a := range inv["argv"].([]any) {
		argv = append(argv, a.(string))
	}
	if !reflect.DeepEqual(argv[1:5], []string{"--resume", id, "--dangerously-skip-permissions", "--settings"}) {
		t.Errorf("argv = %q", argv)
	}
	if resolved(inv["cwd"].(string)) != resolved(cwd) {
		t.Errorf("cwd = %v, want %s", inv["cwd"], cwd)
	}
	h.WaitState(2, session.Running, 5*time.Second)
	if i := h.Slot(2); i.SessionID != id || !i.Resumed || i.Task != "quota" {
		t.Errorf("slot 2 = %+v", i)
	}

	code, body = h.Do("POST", "/api/sessions", map[string]any{"task": "again", "resume_id": id})
	if code != 400 || !strings.Contains(string(body), "already open in slot 2") {
		t.Fatalf("second resume = %d %s", code, body)
	}
	if code, body := h.Do("GET", "/api/transcript?id="+id, nil); code != 200 || !strings.Contains(string(body), `"open_slot":2`) {
		t.Fatalf("lookup while open = %d %s", code, body)
	}
}

func TestResumeByIDRefusesUnknownOrMovedSessions(t *testing.T) {
	gone := uuid.NewString()
	h := tu.Start(t, tu.Opts{Setup: func(home string) {
		writeTranscript(t, home, gone, filepath.Join(home, "deleted-worktree"))
	}})
	for in, want := range map[string]string{
		"nonsense":        session.MsgBadSessionID,
		uuid.NewString(): "was found on this host",
		gone:             "no longer exists",
	} {
		code, body := h.Do("POST", "/api/sessions", map[string]any{"task": "t", "resume_id": in})
		if code != 400 || !strings.Contains(string(body), want) {
			t.Errorf("resume %q = %d %s", in, code, body)
		}
	}
}
