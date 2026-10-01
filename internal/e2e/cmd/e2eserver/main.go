// Command e2eserver runs a real controller for the Playwright acceptance tests:
// fake Tailscale (owner login, 127.0.0.1), fake claude, a temp HOME with git
// repos Documents/repo1..N and Documents/folio. It prints {"url","dir"} as its
// first stdout line. Test-only: it is never part of a release.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ryabinski-labs/claude-code-controller/internal/app"
	"github.com/ryabinski-labs/claude-code-controller/internal/config"
	"github.com/ryabinski-labs/claude-code-controller/internal/testutil"
	"github.com/ryabinski-labs/claude-code-controller/web"
)

func main() {
	n := flag.Int("slots-folders", 4, "number of Documents/repoN git repos to create")
	flag.Parse()
	self, _ := os.Executable()
	binDir := filepath.Dir(self)
	dir, err := os.MkdirTemp("", "ccctl-e2e-")
	must(err)
	dir, _ = filepath.EvalSymlinks(dir)
	home := filepath.Join(dir, "home")
	for i := 1; i <= *n; i++ {
		gitRepo(filepath.Join(home, "Documents", fmt.Sprintf("repo%d", i)))
	}
	gitRepo(filepath.Join(home, "Documents", "folio"))
	fakeLog := filepath.Join(dir, "fakeclaude")
	cfg := config.Config{Port: 0}
	cfg.ApplyDefaults(home, func(string) (string, error) { return filepath.Join(binDir, "fakeclaude"), nil })
	cfg.Port = 0 // ephemeral, never the real 7681
	env := append(os.Environ(), "FAKECLAUDE_LOG="+fakeLog, "HOME="+home)
	a, err := app.New(app.Options{
		Home: home, Config: cfg, TS: testutil.NewFakeTS(), Interval: 100 * time.Millisecond,
		CCCTLPath: filepath.Join(binDir, "ccctl"), Env: env, Static: web.Dist(),
		Log:      slog.New(slog.NewJSONHandler(os.Stderr, nil)),
		SockPath: filepath.Join(dir, "s.sock"),
	})
	must(err)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		for a.Addr() == "" {
			time.Sleep(10 * time.Millisecond)
		}
		b, _ := json.Marshal(map[string]string{"url": "http://" + a.Addr(), "dir": dir})
		fmt.Println(string(b))
	}()
	must(a.Run(ctx))
}

func gitRepo(d string) {
	must(os.MkdirAll(d, 0o755))
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", d}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=e2e", "GIT_AUTHOR_EMAIL=e2e@example", "GIT_COMMITTER_NAME=e2e", "GIT_COMMITTER_EMAIL=e2e@example")
		if b, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "git %v: %v %s\n", args, err, b)
			os.Exit(1)
		}
	}
	run("init", "-q", "-b", "main")
	must(os.WriteFile(filepath.Join(d, "README.md"), []byte("e2e\n"), 0o644))
	run("add", ".")
	run("commit", "-q", "-m", "init")
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
