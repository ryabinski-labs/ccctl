// Command ccctl runs and manages the Claude Code controller.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/ryabinski-labs/claude-code-controller/internal/app"
	"github.com/ryabinski-labs/claude-code-controller/internal/config"
	"github.com/ryabinski-labs/claude-code-controller/internal/hook"
	"github.com/ryabinski-labs/claude-code-controller/internal/install"
	"github.com/ryabinski-labs/claude-code-controller/internal/logx"
	"github.com/ryabinski-labs/claude-code-controller/internal/statuscmd"
	"github.com/ryabinski-labs/claude-code-controller/internal/tailscale"
	"github.com/ryabinski-labs/claude-code-controller/web"
)

var version = "dev"

const usage = `Usage: ccctl <command>

Commands:
  serve               run the controller (normally started by the service manager)
  install [--version vX.Y.Z] [--local]
                      install or upgrade ccctl as a launchd agent / systemd user service
  uninstall           remove the service (keeps ~/.ccctl and every worktree)
  status              print the controller URL (exit 3 when Tailscale is not running)
  hook --slot N       Claude Code hook entry point (internal)
  version             print the version
`

func main() { os.Exit(run(os.Args[1:])) }

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	switch args[0] {
	case "serve":
		return serve(home)
	case "status":
		cfg, err := config.Load(home)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		ts := tailscale.AutoCLI{Configured: cfg.TailscalePath, LookPath: exec.LookPath, Exists: exists}
		return statuscmd.Run(context.Background(), ts, cfg.Port, filepath.Join(config.Dir(home), "ccctl.sock"), os.Stdout)
	case "hook":
		fs := flag.NewFlagSet("hook", flag.ContinueOnError)
		slot := fs.Int("slot", 0, "slot number")
		if err := fs.Parse(args[1:]); err != nil || *slot < 1 || *slot > config.MaxSessions {
			return 0 // never fail Claude
		}
		hook.Run(*slot, os.Stdin)
		return 0
	case "install":
		fs := flag.NewFlagSet("install", flag.ContinueOnError)
		ver := fs.String("version", "", "release tag to install (default latest)")
		local := fs.Bool("local", false, "install this binary instead of downloading")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if err := install.Run(context.Background(), install.Options{Version: *ver, Local: *local, Out: os.Stdout}); err != nil {
			fmt.Fprintln(os.Stderr, "install failed:", err)
			return 1
		}
		firewallHint(home)
		// The service needs a moment to start; poll status for up to 10 s.
		var code int
		for i := 0; i < 20; i++ {
			if code = statusQuiet(home); code == statuscmd.ExitOK {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		return run([]string{"status"})
	case "uninstall":
		if err := install.Uninstall(context.Background(), install.Options{Out: os.Stdout}); err != nil {
			fmt.Fprintln(os.Stderr, "uninstall failed:", err)
			return 1
		}
		return 0
	case "version", "--version":
		fmt.Println(version)
		return 0
	case "help", "-h", "--help":
		fmt.Print(usage)
		return 0
	}
	fmt.Fprint(os.Stderr, usage)
	return 2
}

// firewallHint explains the one-time firewall step on macOS: with the
// Application Firewall on, connections to a new unsigned binary are held
// silently until it is allowed (found on mac-mini).
func firewallHint(home string) {
	const fw = "/usr/libexec/ApplicationFirewall/socketfilterfw"
	if runtime.GOOS != "darwin" {
		return
	}
	out, err := exec.Command(fw, "--getglobalstate").Output()
	if err != nil || !strings.Contains(string(out), "enabled") {
		return
	}
	bin := filepath.Join(config.Dir(home), "bin", "ccctl")
	fmt.Printf("\nThe macOS firewall is on. Allow ccctl to accept connections (once per install or upgrade):\n"+
		"  sudo %[1]s --add %[2]s && sudo %[1]s --unblockapp %[2]s\n\n", fw, bin)
}

func statusQuiet(home string) int {
	cfg, err := config.Load(home)
	if err != nil {
		return 1
	}
	ts := tailscale.AutoCLI{Configured: cfg.TailscalePath, LookPath: exec.LookPath, Exists: exists}
	return statuscmd.Run(context.Background(), ts, cfg.Port, filepath.Join(config.Dir(home), "ccctl.sock"), io.Discard)
}

func serve(home string) int {
	log, closer, err := logx.New(filepath.Join(config.Dir(home), "logs"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer closer.Close()
	cfg, err := config.Load(home)
	if err != nil {
		log.Error("config_invalid", "error", err.Error())
		return 1
	}
	self, _ := os.Executable()
	a, err := app.New(app.Options{
		Home: home, Config: cfg, CCCTLPath: self, Env: os.Environ(), Log: log, Static: web.Dist(),
		TS: tailscale.AutoCLI{Configured: cfg.TailscalePath, LookPath: exec.LookPath, Exists: exists},
	})
	if err != nil {
		log.Error("start_failed", "error", err.Error())
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Info("ccctl_start", "event", "ccctl_start", "version", version)
	if err := a.Run(ctx); err != nil {
		log.Error("run_failed", "error", err.Error())
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
