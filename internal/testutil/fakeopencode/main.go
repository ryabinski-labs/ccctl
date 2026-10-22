// Command fakeopencode stands in for the OpenCode TUI in tests. It prints a
// banner (FAKE_OPENCODE cwd=... slot=... argv=... prompt=...), logs argv and
// environment to $FAKEOPENCODE_LOG/invocations.jsonl, keeps the terminal in raw
// mode, echoes typed lines as "ECHO <line>", and handles:
//
//	!notify  posts permission.asked for its session to CCCTL_SOCK
//	!stop    posts session.idle for its session to CCCTL_SOCK
//
// Modes ($FAKEOPENCODE_MODE_<slot> or $FAKEOPENCODE_MODE):
// exit=N, resume-fail, lines=N.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
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
	logDir := os.Getenv("FAKEOPENCODE_LOG")
	mode := os.Getenv("FAKEOPENCODE_MODE_" + slot)
	if mode == "" {
		mode = os.Getenv("FAKEOPENCODE_MODE")
	}
	args := os.Args[1:]
	var sessionID, prompt string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session":
			i++
			sessionID = args[i]
		case "--prompt":
			i++
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
		k, v, _ := strings.Cut(m, "=")
		n, _ := strconv.Atoi(v)
		switch k {
		case "resume-fail":
			time.Sleep(200 * time.Millisecond)
			fmt.Printf("No OpenCode session: %s\r\n", sessionID)
			os.Exit(1)
		case "exit":
			time.Sleep(200 * time.Millisecond)
			os.Exit(n)
		}
	}
	if st, err := term.MakeRaw(0); err == nil {
		defer term.Restore(0, st)
	}
	fmt.Printf("FAKE_OPENCODE cwd=%s slot=%s argv=%s prompt=%s\r\n", cwd, slot, strings.Join(args, " "), prompt)
	for _, m := range strings.Split(mode, ",") {
		k, v, _ := strings.Cut(m, "=")
		n, _ := strconv.Atoi(v)
		if k == "lines" {
			for i := 1; i <= n; i++ {
				fmt.Printf("slot=%s line=%d\r\n", slot, i)
			}
		}
	}
	buf := make([]byte, 4096)
	var line []byte
	in, _ := os.OpenFile(filepath.Join(logDir, "input-"+slot+".bin"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 && in != nil {
			in.Write(buf[:n])
			in.Sync()
		}
		for _, c := range buf[:n] {
			if c == '\r' || c == '\n' {
				handle(string(line), slot, sessionID)
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

func handle(line, slot, sessionID string) {
	switch line {
	case "!notify":
		postHook(slot, sessionID, "permission.asked")
		fmt.Print("HOOK_OK permission.asked\r\n")
	case "!stop":
		postHook(slot, sessionID, "session.idle")
		fmt.Print("HOOK_OK session.idle\r\n")
	default:
		fmt.Printf("ECHO %s\r\n", line)
	}
}

func postHook(slot, sessionID, event string) {
	n, _ := strconv.Atoi(slot)
	body, _ := json.Marshal(map[string]any{"slot": n, "hook": event, "session_id": sessionID, "source": "opencode-plugin"})
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", os.Getenv("CCCTL_SOCK"))
		},
	}}
	resp, err := client.Post("http://ccctl/hook", "application/json", bytes.NewReader(body))
	if err == nil {
		resp.Body.Close()
	}
}
