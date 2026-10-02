// Package app wires the controller: Tailscale monitor, listener, Unix socket, sessions.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ryabinski-labs/ccctl/internal/auth"
	"github.com/ryabinski-labs/ccctl/internal/config"
	"github.com/ryabinski-labs/ccctl/internal/hook"
	"github.com/ryabinski-labs/ccctl/internal/logx"
	"github.com/ryabinski-labs/ccctl/internal/server"
	"github.com/ryabinski-labs/ccctl/internal/session"
	"github.com/ryabinski-labs/ccctl/internal/state"
	"github.com/ryabinski-labs/ccctl/internal/tailscale"
)

type Options struct {
	Home        string
	Config      config.Config
	TS          tailscale.Client
	Interval    time.Duration
	CCCTLPath   string
	Env         []string
	Log         *slog.Logger
	Static      fs.FS
	StopGrace   time.Duration
	ResumeGrace time.Duration
	// ScanTimeout overrides the 10 s repo scan limit (tests).
	ScanTimeout time.Duration
	// SockPath overrides ~/.ccctl/ccctl.sock (tests use short paths).
	SockPath string
	// Version is the ccctl build version shown to pages.
	Version string
}

type App struct {
	opt   Options
	M     *session.Manager
	S     *server.Server
	Store *state.Store
	mon   *tailscale.Monitor

	mu          sync.Mutex
	st          tailscale.Status
	srv         *http.Server
	ln          net.Listener
	boundAt     time.Time
	message     string
	lastBindErr string // only touched by the check loop
}

func New(o Options) (*App, error) {
	if o.Interval == 0 {
		o.Interval = tailscale.DefaultInterval
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	dir := config.Dir(o.Home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if o.SockPath == "" {
		o.SockPath = filepath.Join(dir, "ccctl.sock")
	}
	if o.CCCTLPath == "" {
		o.CCCTLPath, _ = os.Executable()
	}
	a := &App{opt: o, Store: state.NewStore(dir), mon: tailscale.NewMonitor()}
	a.mon.Interval = o.Interval
	a.S = &server.Server{Home: o.Home, Host: a.hostName, Static: o.Static, Log: o.Log, Version: o.Version, ScanTimeout: o.ScanTimeout}
	a.M = session.NewManager(session.Options{
		Home: o.Home, ClaudePath: o.Config.ClaudePath, CCCTLPath: o.CCCTLPath, SettingsDir: filepath.Join(dir, "sessions"),
		SockPath: o.SockPath, Env: o.Env, Store: a.Store, Log: o.Log, StopGrace: o.StopGrace, ResumeGrace: o.ResumeGrace,
		OnChange: a.S.SlotChanged,
		SessionEnv: func() map[string]string {
			if c, err := config.Load(o.Home); err == nil && c.Env != nil {
				return c.Env
			}
			return o.Config.Env
		},
	})
	a.S.M = a.M
	a.S.Guard = &auth.Guard{Cache: auth.NewCache(o.TS.WhoIs), Allow: a.allowlist, Hosts: a.hosts, Log: o.Log}
	return a, nil
}

func (a *App) hostName() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if short, _, _ := strings.Cut(a.st.DNSName, "."); short != "" {
		return short // MagicDNS name, e.g. mac-mini
	}
	if a.st.HostName != "" {
		return a.st.HostName
	}
	h, _ := os.Hostname()
	return h
}

// Allowlist is allowed_logins, else the host node's own login (A-006).
func (a *App) allowlist() []string {
	if len(a.opt.Config.AllowedLogins) > 0 {
		return a.opt.Config.AllowedLogins
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.st.SelfLogin == "" {
		return nil
	}
	return []string{a.st.SelfLogin}
}

// EffectiveAllowlist exposes the allowlist decision for config + status (SC-005-f).
func EffectiveAllowlist(c config.Config, st tailscale.Status) []string {
	if len(c.AllowedLogins) > 0 {
		return c.AllowedLogins
	}
	if st.SelfLogin == "" {
		return nil
	}
	return []string{st.SelfLogin}
}

func (a *App) hosts() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var hs []string
	short, _, _ := strings.Cut(a.st.DNSName, ".")
	for _, h := range []string{a.st.IPv4, a.st.HostName, a.st.DNSName, short} {
		if h != "" {
			hs = append(hs, h)
		}
	}
	if a.ln != nil {
		hs = append(hs, a.ln.Addr().String())
	}
	return hs
}

// Config returns the controller's configuration.
func (a *App) Config() config.Config { return a.opt.Config }

// Addr is the bound listener address, or "" when not serving.
func (a *App) Addr() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ln == nil {
		return ""
	}
	return a.ln.Addr().String()
}

// BoundAt is when the listener last bound.
func (a *App) BoundAt() time.Time { a.mu.Lock(); defer a.mu.Unlock(); return a.boundAt }

// Message is the current Tailscale problem text ("" when serving).
func (a *App) Message() string { a.mu.Lock(); defer a.mu.Unlock(); return a.message }

// Run resumes saved sessions, then serves until ctx ends (REQ-006, REQ-009).
func (a *App) Run(ctx context.Context) error {
	lock, err := a.lock()
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := a.M.Resume(); err != nil {
		return err
	}
	sock, err := a.serveSock()
	if err != nil {
		return err
	}
	defer func() { sock.Close(); os.Remove(a.opt.SockPath) }() // lock still held here
	a.check(ctx)
	t := time.NewTicker(a.opt.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			a.unbind()
			a.M.Shutdown(5 * time.Second)
			return nil
		case <-t.C:
			a.check(ctx)
		}
	}
}

func (a *App) check(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	st, err := a.opt.TS.Status(cctx)
	cancel()
	if err != nil && st.BackendState == "" {
		st.BackendState = "Unknown"
	}
	a.mu.Lock()
	prevMsg := a.message
	if st.BackendState == "Running" {
		a.st = st
	}
	a.mu.Unlock()
	switch a.mon.Observe(st.BackendState) {
	case tailscale.Bind:
		if err := a.bind(st); err != nil {
			if err.Error() != a.lastBindErr {
				a.opt.Log.Error("bind_failed", "event", "bind_failed", "error", err.Error())
			}
			a.lastBindErr = err.Error()
			a.mon.Observe("BindFailed")
		} else {
			a.lastBindErr = ""
		}
	case tailscale.Unbind:
		// Log before closing so the event always precedes the observable unbind.
		logx.Event(a.opt.Log, "tailscale_down", "backend_state", st.BackendState)
		a.unbind()
	}
	msg := ""
	if st.BackendState != "Running" {
		msg = tailscale.NotRunningMessage
		if err != nil && st.BackendState == "NoCLI" {
			msg = err.Error()
		}
	}
	a.mu.Lock()
	a.message = msg
	a.mu.Unlock()
	if msg != "" && msg != prevMsg {
		errText := ""
		if err != nil {
			errText = err.Error()
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				errText += ": " + strings.TrimSpace(string(ee.Stderr))
			}
		}
		a.opt.Log.Warn("tailscale_not_running", "backend_state", st.BackendState, "message", msg, "error", errText)
	}
}

func (a *App) bind(st tailscale.Status) error {
	if st.IPv4 == "" {
		return errors.New("tailscale reports no IPv4 address")
	}
	ln, err := net.Listen("tcp4", net.JoinHostPort(st.IPv4, strconv.Itoa(a.opt.Config.Port)))
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: a.S.Handler(), ReadHeaderTimeout: 10 * time.Second}
	a.mu.Lock()
	a.ln, a.srv, a.boundAt = ln, srv, time.Now()
	a.mu.Unlock()
	a.opt.Log.Info("listening", "event", "listening", "url", "http://"+ln.Addr().String())
	go srv.Serve(ln)
	return nil
}

func (a *App) unbind() {
	a.mu.Lock()
	srv := a.srv
	a.srv, a.ln = nil, nil
	a.mu.Unlock()
	if srv != nil {
		srv.Close()
		a.S.CloseClients()
	}
}

// AlreadyRunningMessage is returned when another controller holds the lock.
const AlreadyRunningMessage = "ccctl is already running for this user. Stop it first (ccctl uninstall, or stop the other ccctl serve)."

// lock takes an exclusive lock on the socket path so two controllers never share
// one hook socket and state file.
func (a *App) lock() (*os.File, error) {
	f, err := os.OpenFile(a.opt.SockPath+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New(AlreadyRunningMessage)
	}
	return f, nil
}

// StatusReply is GET /status on the Unix socket.
type StatusReply struct {
	URL     string `json:"url"`
	Message string `json:"message"`
}

func (a *App) serveSock() (net.Listener, error) {
	os.Remove(a.opt.SockPath)
	ln, err := net.Listen("unix", a.opt.SockPath)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", a.opt.SockPath, err)
	}
	os.Chmod(a.opt.SockPath, 0o600)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /hook", func(w http.ResponseWriter, r *http.Request) {
		var ev hook.Event
		if json.NewDecoder(r.Body).Decode(&ev) != nil {
			http.Error(w, "bad hook", 400)
			return
		}
		_ = a.M.Hook(ev.Slot, ev.Hook, ev.SessionID)
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		rep := StatusReply{Message: a.Message()}
		if ad := a.Addr(); ad != "" {
			rep.URL = "http://" + ad
		}
		json.NewEncoder(w).Encode(rep)
	})
	go http.Serve(ln, mux)
	return ln, nil
}
