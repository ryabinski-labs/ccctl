// Package hook implements `ccctl hook --slot N`: Claude Code runs it for the
// Notification and Stop hooks, and it forwards the event over the controller's
// Unix socket (A-002). It never blocks or fails Claude: errors exit 0.
package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Event is the body posted to the controller socket.
type Event struct {
	Slot      int    `json:"slot"`
	Hook      string `json:"hook"`
	SessionID string `json:"session_id"`
}

// SockPath returns $CCCTL_SOCK or ~/.ccctl/ccctl.sock.
func SockPath() string {
	if p := os.Getenv("CCCTL_SOCK"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ccctl", "ccctl.sock")
}

// UnixClient returns an HTTP client that dials sock.
func UnixClient(sock string, timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sock)
		}}}
}

// Run reads the hook payload from stdin and posts it for slot.
func Run(slot int, stdin io.Reader) {
	var payload struct {
		HookEventName string `json:"hook_event_name"`
		SessionID     string `json:"session_id"`
	}
	b, _ := io.ReadAll(io.LimitReader(stdin, 1<<20))
	_ = json.Unmarshal(b, &payload)
	ev := Event{Slot: slot, Hook: payload.HookEventName, SessionID: payload.SessionID}
	if ev.Hook == "" {
		ev.Hook = "Notification"
	}
	body, _ := json.Marshal(ev)
	resp, err := UnixClient(SockPath(), 2*time.Second).Post("http://ccctl/hook", "application/json", bytes.NewReader(body))
	if err == nil {
		resp.Body.Close()
	}
}
