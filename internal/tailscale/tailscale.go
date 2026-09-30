// Package tailscale talks to the host's existing tailscaled through its CLI (DL-007).
package tailscale

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// NotRunningMessage is the §5 text shown when Tailscale is not Running.
const NotRunningMessage = "Tailscale is not running on this host. Start the Tailscale app or run: sudo tailscale up. Retrying every 10 seconds."

// CLINotFoundMessage is the A-021 text when no CLI can be located.
const CLINotFoundMessage = "Tailscale CLI not found. Install Tailscale or set tailscale_path in ~/.ccctl/config.toml."

// AppBundlePath is the macOS app-bundle CLI (mac-mini has no tailscale on PATH).
const AppBundlePath = "/Applications/Tailscale.app/Contents/MacOS/Tailscale"

// Status is the subset of `tailscale status --json` the controller uses.
type Status struct {
	BackendState string
	IPv4         string // host's Tailscale IPv4
	HostName     string
	DNSName      string // MagicDNS name without trailing dot
	SelfLogin    string // login of the user owning this node
}

// Client is the host Tailscale daemon; tests substitute a fake.
type Client interface {
	Status(ctx context.Context) (Status, error)
	// WhoIs returns the login name of the tailnet user at remote address addr (ip:port).
	WhoIs(ctx context.Context, addr string) (string, error)
}

// ResolveCLI implements A-021: config path, else PATH, else the macOS app bundle.
func ResolveCLI(configured string, lookPath func(string) (string, error), exists func(string) bool) (string, error) {
	if configured != "" {
		return configured, nil
	}
	if p, err := lookPath("tailscale"); err == nil {
		return p, nil
	}
	if exists(AppBundlePath) {
		return AppBundlePath, nil
	}
	return "", errors.New(CLINotFoundMessage)
}

// CLI runs the tailscale binary at Path.
type CLI struct{ Path string }

type statusJSON struct {
	BackendState string
	Self         struct {
		UserID       int64
		HostName     string
		DNSName      string
		TailscaleIPs []string
	}
	User map[string]struct{ LoginName string }
}

// ParseStatus decodes `tailscale status --json` output.
func ParseStatus(b []byte) (Status, error) {
	var sj statusJSON
	if err := json.Unmarshal(b, &sj); err != nil {
		return Status{}, err
	}
	st := Status{BackendState: sj.BackendState, HostName: sj.Self.HostName, DNSName: strings.TrimSuffix(sj.Self.DNSName, ".")}
	for _, ip := range sj.Self.TailscaleIPs {
		if !strings.Contains(ip, ":") {
			st.IPv4 = ip
			break
		}
	}
	if u, ok := sj.User[fmt.Sprint(sj.Self.UserID)]; ok {
		st.SelfLogin = u.LoginName
	}
	return st, nil
}

func (c CLI) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, c.Path, args...).Output()
}

func (c CLI) Status(ctx context.Context) (Status, error) {
	out, err := c.run(ctx, "status", "--json")
	if len(out) == 0 && err != nil {
		return Status{BackendState: "NoState"}, err
	}
	// status exits non-zero when stopped but still prints JSON.
	return ParseStatus(out)
}

func (c CLI) WhoIs(ctx context.Context, addr string) (string, error) {
	out, err := c.run(ctx, "whois", "--json", addr)
	if err != nil {
		return "", err
	}
	var w struct{ UserProfile struct{ LoginName string } }
	if err := json.Unmarshal(out, &w); err != nil {
		return "", err
	}
	return w.UserProfile.LoginName, nil
}

// Action is what the listener should do after a status check.
type Action int

const (
	NoOp Action = iota
	Bind
	Unbind
)

func (a Action) String() string { return [...]string{"no-op", "bind", "unbind"}[a] }

// DefaultInterval is the check period (A-005).
const DefaultInterval = 10 * time.Second

// Monitor is the bind/unbind state machine driven by BackendState.
type Monitor struct {
	Interval time.Duration
	bound    bool
}

func NewMonitor() *Monitor { return &Monitor{Interval: DefaultInterval} }

// Observe returns the action for a newly observed backend state.
func (m *Monitor) Observe(backendState string) Action {
	running := backendState == "Running"
	switch {
	case running && !m.bound:
		m.bound = true
		return Bind
	case !running && m.bound:
		m.bound = false
		return Unbind
	}
	return NoOp
}

// AutoCLI resolves the CLI on every call so installing Tailscale later is picked up (A-021).
type AutoCLI struct {
	Configured string
	LookPath   func(string) (string, error)
	Exists     func(string) bool
}

func (a AutoCLI) cli() (CLI, error) {
	p, err := ResolveCLI(a.Configured, a.LookPath, a.Exists)
	return CLI{Path: p}, err
}

func (a AutoCLI) Status(ctx context.Context) (Status, error) {
	c, err := a.cli()
	if err != nil {
		return Status{BackendState: "NoCLI"}, err
	}
	return c.Status(ctx)
}

func (a AutoCLI) WhoIs(ctx context.Context, addr string) (string, error) {
	c, err := a.cli()
	if err != nil {
		return "", err
	}
	return c.WhoIs(ctx, addr)
}
