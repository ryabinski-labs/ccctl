// Package testutil is the shared harness for the TDD scenarios: built binaries,
// a fake Tailscale, a controller on 127.0.0.1, a WebSocket client, and log capture.
package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/ryabinski-labs/claude-code-controller/internal/app"
	"github.com/ryabinski-labs/claude-code-controller/internal/config"
	"github.com/ryabinski-labs/claude-code-controller/internal/session"
	"github.com/ryabinski-labs/claude-code-controller/internal/tailscale"
)

// ---- binaries ----

var (
	buildOnce          sync.Once
	binDir, buildError string
)

// RepoRoot is the module root.
func RepoRoot() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "..", "..")
}

// Binaries builds ccctl and fakeclaude once per test process.
func Binaries(t testing.TB) (ccctl, fake string) {
	t.Helper()
	buildOnce.Do(func() {
		d, err := os.MkdirTemp("", "ccctl-bin")
		if err != nil {
			buildError = err.Error()
			return
		}
		binDir = d
		for pkg, out := range map[string]string{"./cmd/ccctl": "ccctl", "./internal/testutil/fakeclaude": "fakeclaude"} {
			cmd := exec.Command("go", "build", "-o", filepath.Join(d, out), pkg)
			cmd.Dir = RepoRoot()
			if b, err := cmd.CombinedOutput(); err != nil {
				buildError = fmt.Sprintf("build %s: %v\n%s", pkg, err, b)
				return
			}
		}
	})
	if buildError != "" {
		t.Fatal(buildError)
	}
	return filepath.Join(binDir, "ccctl"), filepath.Join(binDir, "fakeclaude")
}

// ---- fake Tailscale ----

type FakeTS struct {
	mu         sync.Mutex
	State      string
	IPv4       string
	HostName   string
	SelfLogin  string
	Login      string // login returned by WhoIs for every caller
	WhoIsErr   error
	WhoIsCalls int
}

func NewFakeTS() *FakeTS {
	return &FakeTS{State: "Running", IPv4: "127.0.0.1", HostName: "mac-mini", SelfLogin: "owner@example", Login: "owner@example"}
}

func (f *FakeTS) Set(fn func(f *FakeTS)) { f.mu.Lock(); fn(f); f.mu.Unlock() }

func (f *FakeTS) Status(ctx context.Context) (tailscale.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return tailscale.Status{BackendState: f.State, IPv4: f.IPv4, HostName: f.HostName, SelfLogin: f.SelfLogin}, nil
}

func (f *FakeTS) WhoIs(ctx context.Context, addr string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.WhoIsCalls++
	return f.Login, f.WhoIsErr
}

// ---- log capture ----

type LogBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *LogBuf) Write(p []byte) (int, error) { l.mu.Lock(); defer l.mu.Unlock(); return l.b.Write(p) }

func (l *LogBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

// Events returns parsed JSON log lines whose "event" equals name.
func (l *LogBuf) Events(name string) []map[string]any {
	var out []map[string]any
	for _, line := range strings.Split(l.String(), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil && m["event"] == name {
			out = append(out, m)
		}
	}
	return out
}

// ---- git ----

// GitRepo creates a repo with one commit at dir and returns HEAD.
func GitRepo(t testing.TB, dir string) string {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	run := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
		return strings.TrimSpace(string(b))
	}
	run("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("x\n"), 0o644)
	run("add", ".")
	run("commit", "-q", "-m", "init")
	return run("rev-parse", "HEAD")
}

// Git runs git -C dir args and returns trimmed output.
func Git(dir string, args ...string) (string, error) {
	b, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	return strings.TrimSpace(string(b)), err
}

// ---- controller harness ----

type Opts struct {
	Interval    time.Duration
	Modes       map[int]string // FAKECLAUDE_MODE_<slot>
	Mode        string
	StopGrace   time.Duration
	ResumeGrace time.Duration
	ClaudePath  string // override (default fakeclaude)
	Setup       func(home string)
	TS          *FakeTS
	NoWaitBind  bool
	Allowed     []string
}

type H struct {
	T       testing.TB
	App     *app.App
	TS      *FakeTS
	Home    string
	Root    string // HOME/Documents
	FakeLog string
	Log     *LogBuf
	CCCTL   string
	Sock    string
	cancel  context.CancelFunc
	done    chan struct{}
}

// Start runs a controller with fakeclaude and a fake Tailscale on 127.0.0.1:0.
func Start(t testing.TB, o Opts) *H {
	t.Helper()
	ccctl, fake := Binaries(t)
	home := t.TempDir()
	home, _ = filepath.EvalSymlinks(home)
	root := filepath.Join(home, "Documents")
	os.MkdirAll(root, 0o755)
	if o.Setup != nil {
		o.Setup(home)
	}
	if o.TS == nil {
		o.TS = NewFakeTS()
	}
	if o.Interval == 0 {
		o.Interval = 50 * time.Millisecond
	}
	fakeLog := filepath.Join(home, "fakeclaude")
	env := append(os.Environ(), "FAKECLAUDE_LOG="+fakeLog, "FAKECLAUDE_MODE="+o.Mode)
	for s, m := range o.Modes {
		env = append(env, fmt.Sprintf("FAKECLAUDE_MODE_%d=%s", s, m))
	}
	cfg := config.Config{Port: 0, AllowedLogins: o.Allowed}
	cfg.ApplyDefaults(home, func(string) (string, error) { return fake, nil })
	cfg.Port = 0 // ephemeral, never the real 7681
	if o.ClaudePath != "" {
		cfg.ClaudePath = o.ClaudePath
	}
	sockDir, _ := os.MkdirTemp("", "cc")
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	lb := &LogBuf{}
	a, err := app.New(app.Options{
		Home: home, Config: cfg, TS: o.TS, Interval: o.Interval, CCCTLPath: ccctl, Env: env,
		Log: slog.New(slog.NewJSONHandler(lb, nil)), StopGrace: o.StopGrace, ResumeGrace: o.ResumeGrace,
		SockPath: filepath.Join(sockDir, "s.sock"),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &H{Sock: filepath.Join(sockDir, "s.sock"), T: t, App: a, TS: o.TS, Home: home, Root: root, FakeLog: fakeLog, Log: lb, CCCTL: ccctl, cancel: cancel, done: make(chan struct{})}
	go func() { a.Run(ctx); close(h.done) }()
	t.Cleanup(h.Stop)
	if !o.NoWaitBind {
		Eventually(t, 5*time.Second, func() bool { return a.Addr() != "" }, "controller never bound")
	}
	return h
}

// Stop shuts the controller down (idempotent).
func (h *H) Stop() {
	h.cancel()
	select {
	case <-h.done:
	case <-time.After(10 * time.Second):
	}
}

// URL is the base http URL.
func (h *H) URL() string { return "http://" + h.App.Addr() }

// Do sends a request with the controller's own Origin.
func (h *H) Do(method, path string, body any) (int, []byte) {
	h.T.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.URL()+path, r)
	req.Header.Set("Origin", h.URL())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.T.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// Launch launches a session and fails the test unless it is accepted.
func (h *H) Launch(task, path string, worktree bool, prompt string) session.Info {
	h.T.Helper()
	code, b := h.Do("POST", "/api/sessions", session.LaunchRequest{Task: task, Path: path, Worktree: worktree, Prompt: prompt})
	if code != 201 {
		h.T.Fatalf("launch %s: %d %s", task, code, b)
	}
	var r struct{ Info session.Info }
	json.Unmarshal(b, &r)
	return r.Info
}

// Invocations returns fakeclaude invocation records.
func (h *H) Invocations() []map[string]any {
	b, _ := os.ReadFile(filepath.Join(h.FakeLog, "invocations.jsonl"))
	var out []map[string]any
	for _, l := range strings.Split(string(b), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

// InputLog returns the bytes fakeclaude in slot read.
func (h *H) InputLog(slot int) []byte {
	b, _ := os.ReadFile(filepath.Join(h.FakeLog, fmt.Sprintf("input-%d.bin", slot)))
	return b
}

// Slot returns the manager's view of slot.
func (h *H) Slot(slot int) *session.Info { return h.App.M.Slots()[slot-1] }

// WaitState waits until slot has state st.
func (h *H) WaitState(slot int, st session.State, d time.Duration) {
	h.T.Helper()
	Eventually(h.T, d, func() bool { i := h.Slot(slot); return i != nil && i.State == st }, fmt.Sprintf("slot %d never reached %s (now %+v)", slot, st, h.Slot(slot)))
}

// Eventually polls cond every 10 ms until d.
func Eventually(t testing.TB, d time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}

// ---- WebSocket client ----

type WS struct {
	t      testing.TB
	C      *websocket.Conn
	mu     sync.Mutex
	out    map[int]*bytes.Buffer
	Texts  []map[string]any
	Frames []Frame
}

type Frame struct {
	Type byte
	Slot int
	Data []byte
	At   time.Time
}

// Dial opens the WebSocket with the controller's own Origin.
func (h *H) Dial() *WS {
	h.T.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws://"+h.App.Addr()+"/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {h.URL()}}})
	if err != nil {
		h.T.Fatalf("dial: %v", err)
	}
	c.SetReadLimit(8 << 20)
	w := &WS{t: h.T, C: c, out: map[int]*bytes.Buffer{}}
	go w.loop()
	h.T.Cleanup(func() { c.CloseNow() })
	return w
}

func (w *WS) loop() {
	for {
		typ, data, err := w.C.Read(context.Background())
		if err != nil {
			return
		}
		now := time.Now()
		w.mu.Lock()
		if typ == websocket.MessageText {
			var m map[string]any
			json.Unmarshal(data, &m)
			w.Texts = append(w.Texts, m)
		} else if len(data) >= 2 {
			s := int(data[1])
			if w.out[s] == nil {
				w.out[s] = &bytes.Buffer{}
			}
			w.out[s].Write(data[2:])
			w.Frames = append(w.Frames, Frame{data[0], s, data[2:], now})
		}
		w.mu.Unlock()
	}
}

// Output returns everything received for slot (replay + live).
func (w *WS) Output(slot int) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if b := w.out[slot]; b != nil {
		return b.String()
	}
	return ""
}

// FramesCopy returns a snapshot of binary frames.
func (w *WS) FramesCopy() []Frame {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]Frame(nil), w.Frames...)
}

// TextsCopy returns a snapshot of text frames.
func (w *WS) TextsCopy() []map[string]any {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]map[string]any(nil), w.Texts...)
}

// WaitOutput waits until slot's output contains s.
func (w *WS) WaitOutput(slot int, s string, d time.Duration) {
	w.t.Helper()
	Eventually(w.t, d, func() bool { return strings.Contains(w.Output(slot), s) }, fmt.Sprintf("slot %d output never contained %q; got %q", slot, s, tail(w.Output(slot))))
}

func tail(s string) string {
	if len(s) > 400 {
		return s[len(s)-400:]
	}
	return s
}

// Input sends bytes to slot.
func (w *WS) Input(slot int, data string) {
	if err := w.C.Write(context.Background(), websocket.MessageBinary, append([]byte{1, byte(slot)}, data...)); err != nil {
		w.t.Fatalf("input: %v", err)
	}
}

// Resize sends a resize frame.
func (w *WS) Resize(slot int, rows, cols int) {
	b, _ := json.Marshal(map[string]any{"type": "resize", "slot": slot, "rows": rows, "cols": cols})
	if err := w.C.Write(context.Background(), websocket.MessageText, b); err != nil {
		w.t.Fatalf("resize: %v", err)
	}
}
