// Access, Tailscale, and listener scenarios (REQ-005, REQ-006).
package tdd

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/ryabinski-labs/ccctl/internal/app"
	"github.com/ryabinski-labs/ccctl/internal/server"
	"github.com/ryabinski-labs/ccctl/internal/session"
	"github.com/ryabinski-labs/ccctl/internal/statuscmd"
	tu "github.com/ryabinski-labs/ccctl/internal/testutil"
)

func asOther(f *tu.FakeTS) { f.Login = "other@example" }

// SC-005-b
func TestSc005BUnlistedLoginGetsThe403PageAndAnAuditLine(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	h.TS.Set(asOther)
	code, body := h.Do("GET", "/", nil)
	if code != 403 || !strings.Contains(string(body), "Not authorized: other@example is not on this controller&#39;s allowlist.") &&
		!strings.Contains(string(body), "Not authorized: other@example is not on this controller's allowlist.") {
		t.Fatalf("got %d %s", code, body)
	}
	ev := h.Log.Events("auth_denied")
	if len(ev) == 0 || ev[0]["login"] != "other@example" || ev[0]["path"] != "/" {
		t.Fatalf("auth_denied events = %v", ev)
	}
}

func dialStatus(t *testing.T, h *tu.H, origin string) int {
	hdr := http.Header{}
	if origin != "" {
		hdr.Set("Origin", origin)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, "ws://"+h.App.Addr()+"/ws", &websocket.DialOptions{HTTPHeader: hdr})
	if err == nil {
		c.CloseNow()
		return 101
	}
	if resp == nil {
		t.Fatalf("dial: %v", err)
	}
	return resp.StatusCode
}

// SC-005-c
func TestSc005CUnlistedLoginCannotOpenTheWebsocket(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	d := filepath.Join(h.Root, "s")
	os.MkdirAll(d, 0o755)
	if _, err := h.App.M.Launch(session.LaunchRequest{Task: "s", Path: d}); err != nil {
		t.Fatal(err)
	}
	h.TS.Set(asOther)
	if st := dialStatus(t, h, h.URL()); st != 403 {
		t.Fatalf("upgrade status = %d", st)
	}
	time.Sleep(300 * time.Millisecond)
	if b := h.InputLog(1); len(b) != 0 {
		t.Fatalf("session read %q", b)
	}
}

// SC-005-d
func TestSc005DForeignOrMissingOriginIsRefused(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	for _, o := range []string{"https://evil.example", ""} {
		if st := dialStatus(t, h, o); st != 403 {
			t.Errorf("Origin %q: status %d", o, st)
		}
	}
}

// SC-005-e
func TestSc005EListenerBindsOnlyToTheTailscaleIpv4(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	host, port, _ := net.SplitHostPort(h.App.Addr())
	if host != "127.0.0.1" {
		t.Fatalf("listener = %s", h.App.Addr())
	}
	var other string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLoopback() {
			other = ipn.IP.String()
			break
		}
	}
	if other == "" {
		t.Skip("host has no non-loopback IPv4 address")
	}
	c, err := net.DialTimeout("tcp", net.JoinHostPort(other, port), 2*time.Second)
	if err == nil {
		c.Close()
		t.Fatalf("%s:%s accepted a connection", other, port)
	}
	if !strings.Contains(err.Error(), "refused") {
		t.Fatalf("dial %s: %v (want connection refused)", other, err)
	}
}

// SC-005-h
func TestSc005HEveryRouteEnforcesTheAllowlist(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	h.TS.Set(asOther)
	for _, r := range server.Routes {
		if code, _ := h.Do(r.Method, r.Path, nil); code != 403 {
			t.Errorf("%s %s = %d", r.Method, r.Path, code)
		}
	}
}

// SC-006-c
func TestSc006CStartupWithTailscaleStopped(t *testing.T) {
	ts := tu.NewFakeTS()
	ts.State = "Stopped"
	h := tu.Start(t, tu.Opts{TS: ts, Interval: 100 * time.Millisecond, NoWaitBind: true})
	const msg = "Tailscale is not running on this host. Start the Tailscale app or run: sudo tailscale up. Retrying every 10 seconds."
	time.Sleep(350 * time.Millisecond)
	if a := h.App.Addr(); a != "" {
		t.Fatalf("listening at %s while Tailscale is stopped", a)
	}
	if !strings.Contains(h.Log.String(), msg) {
		t.Errorf("log lacks the message:\n%s", h.Log.String())
	}
	var out bytes.Buffer
	if code := statuscmd.Run(context.Background(), ts, 7681, h.Sock, &out); code != 3 || !strings.Contains(out.String(), msg) {
		t.Errorf("status = %d %q", code, out.String())
	}
	ts.Set(func(f *tu.FakeTS) { f.State = "Running" })
	start := time.Now()
	tu.Eventually(t, time.Second, func() bool { return h.App.Addr() != "" }, "never bound")
	if el := time.Since(start); el > 200*time.Millisecond {
		t.Errorf("bound %v after Tailscale came up", el)
	}
}

// SC-006-d
func TestSc006DTailscaleDropKeepsSessionsAlive(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	launchN(h, 2)
	tu.Eventually(t, 5*time.Second, func() bool { return h.App.M.PID(1) > 0 && h.App.M.PID(2) > 0 }, "sessions not started")
	addr := h.App.Addr()
	h.TS.Set(func(f *tu.FakeTS) { f.State = "Stopped" })
	tu.Eventually(t, 2*time.Second, func() bool { return h.App.Addr() == "" }, "listener still open")
	if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
		c.Close()
		t.Fatal("old address still accepts connections")
	}
	if len(h.Log.Events("tailscale_down")) == 0 {
		t.Error("no tailscale_down log line")
	}
	for s := 1; s <= 2; s++ {
		if err := syscall.Kill(h.App.M.PID(s), 0); err != nil {
			t.Errorf("session %d died: %v", s, err)
		}
	}
	h.TS.Set(func(f *tu.FakeTS) { f.State = "Running" })
	tu.Eventually(t, 2*time.Second, func() bool { return h.App.Addr() != "" }, "never re-bound")
	c, err := net.DialTimeout("tcp", h.App.Addr(), time.Second)
	if err != nil {
		t.Fatalf("dial after recovery: %v", err)
	}
	c.Close()
}

// SC-006-g
func TestSc006GSecondControllerRefusesToStart(t *testing.T) {
	h := tu.Start(t, tu.Opts{})
	b, err := app.New(app.Options{Home: h.Home, Config: h.App.Config(), TS: tu.NewFakeTS(), SockPath: h.Sock, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := b.Run(ctx); err == nil || err.Error() != app.AlreadyRunningMessage {
		t.Fatalf("second Run = %v", err)
	}
	if _, err := os.Stat(h.Sock); err != nil {
		t.Fatalf("first controller's socket was removed: %v", err)
	}
}
