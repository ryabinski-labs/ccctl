// Command fakeclaude stands in for the claude CLI in tests. It prints a banner
// (FAKE_CLAUDE cwd=... slot=... argv=... prompt=...), puts its terminal in raw
// mode, logs every input byte to $FAKECLAUDE_LOG/input-<slot>.bin, and echoes
// each typed line as "ECHO <line>". "!copy TEXT" emits an OSC 52 clipboard write. Lines "!stop" and "!notify" run the hook
// command from the --settings file; "print A B" prints "slot=S line=n" for n in A..B.
//
// Modes ($FAKECLAUDE_MODE_<slot> or $FAKECLAUDE_MODE, comma separated):
// ignore-hup, exit=N, resume-fail, lines=N, ticker=N (N timestamped lines "TS <unixnano> <i>").
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

func main() {
	slot := os.Getenv("CCCTL_SLOT")
	logDir := os.Getenv("FAKECLAUDE_LOG")
	mode := os.Getenv("FAKECLAUDE_MODE_" + slot)
	if mode == "" {
		mode = os.Getenv("FAKECLAUDE_MODE")
	}
	args := os.Args[1:]
	var settings, sessionID, prompt string
	resume := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--settings":
			i++
			settings = args[i]
		case "--session-id":
			i++
			sessionID = args[i]
		case "--resume":
			i++
			sessionID, resume = args[i], true
		case "--dangerously-skip-permissions", "--":
		default:
			prompt = args[i]
		}
	}
	cwd, _ := os.Getwd()
	if logDir != "" {
		os.MkdirAll(logDir, 0o755)
		b, _ := json.Marshal(map[string]any{"slot": slot, "argv": os.Args, "cwd": cwd, "env": os.Environ(), "pid": os.Getpid(), "time": time.Now().UnixNano()})
		f, _ := os.OpenFile(filepath.Join(logDir, "invocations.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		f.Write(append(b, '\n'))
		f.Close()
	}
	signal.Ignore(syscall.SIGINT)
	for _, m := range strings.Split(mode, ",") {
		if m == "ignore-hup" {
			signal.Ignore(syscall.SIGHUP)
		}
		if m == "resume-fail" && resume {
			fmt.Printf("No conversation found with session ID: %s\r\n", sessionID)
			os.Exit(1)
		}
	}
	if st, err := term.MakeRaw(0); err == nil {
		defer term.Restore(0, st)
	}
	fmt.Printf("FAKE_CLAUDE cwd=%s slot=%s argv=%s prompt=%s\r\n", cwd, slot, strings.Join(args, " "), prompt)
	for _, m := range strings.Split(mode, ",") {
		k, v, _ := strings.Cut(m, "=")
		n, _ := strconv.Atoi(v)
		switch k {
		case "exit":
			time.Sleep(200 * time.Millisecond)
			os.Exit(n)
		case "lines":
			printLines(slot, 1, n)
		case "ticker":
			go func() {
				for i := 1; i <= n; i++ {
					fmt.Printf("TS %d %d\r\n", time.Now().UnixNano(), i)
					time.Sleep(time.Millisecond)
				}
			}()
		}
	}
	var in *os.File
	if logDir != "" {
		in, _ = os.OpenFile(filepath.Join(logDir, "input-"+slot+".bin"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	}
	buf := make([]byte, 4096)
	var line []byte
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 && in != nil {
			in.Write(buf[:n])
			in.Sync()
		}
		for _, c := range buf[:n] {
			if c == '\r' || c == '\n' {
				handle(string(line), slot, settings, sessionID)
				line = line[:0]
			} else if c >= 0x20 && c < 0x7f {
				line = append(line, c)
			}
		}
		if err != nil {
			return
		}
	}
}

func printLines(slot string, a, b int) {
	var sb strings.Builder
	for i := a; i <= b; i++ {
		fmt.Fprintf(&sb, "slot=%s line=%d\r\n", slot, i)
	}
	os.Stdout.WriteString(sb.String())
}

func handle(line, slot, settings, sessionID string) {
	switch {
	case line == "!stop":
		runHook(settings, "Stop", sessionID)
	case line == "!notify":
		runHook(settings, "Notification", sessionID)
	case strings.HasPrefix(line, "!copy "):
		// Like Claude Code's selection copy: an OSC 52 clipboard write.
		fmt.Printf("\x1b]52;c;%s\x07COPIED\r\n", base64.StdEncoding.EncodeToString([]byte(strings.TrimPrefix(line, "!copy "))))
	case strings.HasPrefix(line, "print "):
		var a, b int
		fmt.Sscanf(line, "print %d %d", &a, &b)
		printLines(slot, a, b)
	default:
		fmt.Printf("ECHO %s\r\n", line)
	}
}

func runHook(settings, event, sessionID string) {
	b, err := os.ReadFile(settings)
	if err != nil {
		fmt.Printf("HOOK_ERROR %v\r\n", err)
		return
	}
	var s struct {
		Hooks map[string][]struct {
			Hooks []struct{ Command string }
		}
	}
	json.Unmarshal(b, &s)
	if len(s.Hooks[event]) == 0 || len(s.Hooks[event][0].Hooks) == 0 {
		fmt.Printf("HOOK_ERROR no %s hook\r\n", event)
		return
	}
	cmd := exec.Command("sh", "-c", s.Hooks[event][0].Hooks[0].Command)
	payload, _ := json.Marshal(map[string]string{"hook_event_name": event, "session_id": sessionID})
	cmd.Stdin = strings.NewReader(string(payload))
	if err := cmd.Run(); err != nil {
		fmt.Printf("HOOK_ERROR %v\r\n", err)
		return
	}
	fmt.Printf("HOOK_OK %s\r\n", event)
}
