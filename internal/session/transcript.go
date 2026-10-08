package session

import (
	"bufio"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ryabinski-labs/ccctl/internal/config"
)

// Resume-by-ID messages.
const (
	MsgBadSessionID    = "Paste a session ID such as 18cf1881-5565-4e54-b0fd-ffafef9e86b1, or a claude --resume command."
	MsgResumeWithInput = "A resumed session continues its own conversation, so it cannot take a first prompt or a new worktree."
)

func MsgTranscriptNotFound(id string) string {
	return fmt.Sprintf("No Claude Code session %s was found on this host.", id)
}
func MsgTranscriptFolderGone(cwd string) string {
	return fmt.Sprintf("The session's folder %s no longer exists, so it cannot be resumed.", cwd)
}
func MsgAlreadyOpen(id string, slot int) string {
	return fmt.Sprintf("Session %s is already open in slot %d.", id, slot)
}
func MsgOpenElsewhere(id string, pid int) string {
	return fmt.Sprintf("Session %s is open in another terminal (process %d). Quit it there first, then resume it here.", id, pid)
}

var uuidRe = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)

// ParseSessionID pulls one session UUID out of a bare ID or a pasted command
// such as `claude --resume <id>`. More than one distinct UUID is ambiguous.
func ParseSessionID(in string) (string, error) {
	var id string
	for _, m := range uuidRe.FindAllString(in, -1) {
		m = strings.ToLower(m)
		if id != "" && id != m {
			return "", errors.New(MsgBadSessionID)
		}
		id = m
	}
	if id == "" {
		return "", errors.New(MsgBadSessionID)
	}
	return id, nil
}

// Transcript is what ccctl knows about a saved Claude Code session.
type Transcript struct {
	SessionID string    `json:"session_id"`
	Cwd       string    `json:"cwd"`
	Title      string    `json:"title"`
	LastPrompt string    `json:"last_prompt"`
	Branch     string    `json:"branch"`
	Modified   time.Time `json:"modified"`
	// CwdMissing is set when the recorded folder no longer exists.
	CwdMissing bool `json:"cwd_missing"`
	// OpenSlot is the slot already running this session (0 = none).
	OpenSlot int `json:"open_slot"`
	// ExternalPID is a live claude outside ccctl that has this session open (0 = none).
	ExternalPID int `json:"external_pid"`
}

// Transcripts can be hundreds of megabytes; only the ends are read.
const (
	transcriptHead = 2 << 20
	transcriptTail = 1 << 20
)

// FindTranscript locates <configDir>/projects/*/<id>.jsonl and reads its
// working folder (from the first record that has one) and its latest title.
func FindTranscript(configDir, id string) (Transcript, error) {
	matches, _ := filepath.Glob(filepath.Join(configDir, "projects", "*", id+".jsonl"))
	var path string
	var mod time.Time
	for _, p := range matches {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() && fi.ModTime().After(mod) {
			path, mod = p, fi.ModTime()
		}
	}
	if path == "" {
		return Transcript{}, errors.New(MsgTranscriptNotFound(id))
	}
	f, err := os.Open(path)
	if err != nil {
		return Transcript{}, errors.New(MsgTranscriptNotFound(id))
	}
	defer f.Close()
	t := Transcript{SessionID: id, Modified: mod.UTC()}
	type rec struct {
		Type      string `json:"type"`
		Cwd       string `json:"cwd"`
		GitBranch string `json:"gitBranch"`
		AITitle    string `json:"aiTitle"`
		Title      string `json:"customTitle"`
		Summary    string `json:"summary"`
		LastPrompt string `json:"lastPrompt"`
	}
	// A user's custom title (/rename) wins over AI titles, which keep being appended after it.
	var custom, ai, summary string
	title := func(r rec) {
		switch {
		case r.Type == "custom-title" && r.Title != "":
			custom = r.Title
		case r.Type == "ai-title" && r.AITitle != "":
			ai = r.AITitle
		case r.Type == "summary" && r.Summary != "":
			summary = r.Summary
		case r.Type == "last-prompt" && r.LastPrompt != "":
			t.LastPrompt = r.LastPrompt
		}
	}
	scan := func(r io.Reader, each func(rec)) {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64<<10), transcriptHead)
		for sc.Scan() {
			var x rec
			if json.Unmarshal(sc.Bytes(), &x) == nil {
				each(x)
			}
		}
	}
	scan(io.LimitReader(f, transcriptHead), func(r rec) {
		if t.Cwd == "" && r.Cwd != "" {
			t.Cwd, t.Branch = r.Cwd, r.GitBranch
		}
		title(r)
	})
	if fi, err := f.Stat(); err == nil && fi.Size() > transcriptHead {
		off := max(fi.Size()-transcriptTail, transcriptHead)
		if _, err := f.Seek(off, io.SeekStart); err == nil {
			scan(f, func(r rec) { // the first line is usually partial and fails to parse
				if r.GitBranch != "" {
					t.Branch = r.GitBranch
				}
				title(r)
			})
		}
	}
	if t.Cwd == "" {
		return Transcript{}, errors.New(MsgTranscriptNotFound(id))
	}
	t.Title = cmp.Or(custom, ai, summary)
	if r := []rune(t.LastPrompt); len(r) > 200 {
		t.LastPrompt = string(r[:200]) + "…"
	}
	if fi, err := os.Stat(t.Cwd); err != nil || !fi.IsDir() {
		t.CwdMissing = true
	}
	return t, nil
}

// ExternalPID returns the pid of a live claude that has session id open, from the
// registry claude keeps at <configDir>/sessions/<pid>.json (0 = none). An entry whose
// process has exited, or whose pid now belongs to a process started at another time,
// is stale and ignored.
func ExternalPID(configDir, id string) int {
	files, _ := filepath.Glob(filepath.Join(configDir, "sessions", "*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var e struct {
			PID       int    `json:"pid"`
			SessionID string `json:"sessionId"`
			ProcStart string `json:"procStart"`
		}
		if json.Unmarshal(b, &e) != nil || e.PID <= 0 || !strings.EqualFold(e.SessionID, id) {
			continue
		}
		if err := syscall.Kill(e.PID, 0); err != nil && !errors.Is(err, syscall.EPERM) {
			continue
		}
		if e.ProcStart != "" {
			ps := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(e.PID))
			ps.Env = append(os.Environ(), "TZ=UTC") // claude records procStart in UTC
			out, err := ps.Output()
			if err == nil && strings.Join(strings.Fields(string(out)), " ") != strings.Join(strings.Fields(e.ProcStart), " ") {
				continue
			}
		}
		return e.PID
	}
	return 0
}

// claudeConfigDir is where claude keeps transcripts: CLAUDE_CONFIG_DIR from
// the session environment or the controller's own, else ~/.claude.
func (m *Manager) claudeConfigDir() string {
	if m.opt.SessionEnv != nil {
		if v := m.opt.SessionEnv()["CLAUDE_CONFIG_DIR"]; v != "" {
			return config.Expand(m.opt.Home, v)
		}
	}
	for _, e := range m.opt.Env {
		if v, ok := strings.CutPrefix(e, "CLAUDE_CONFIG_DIR="); ok && v != "" {
			return config.Expand(m.opt.Home, v)
		}
	}
	return filepath.Join(m.opt.Home, ".claude")
}

// LookupTranscript resolves pasted input to a saved session, flagging one
// that is already open in a slot.
func (m *Manager) LookupTranscript(in string) (Transcript, error) {
	id, err := ParseSessionID(in)
	if err != nil {
		return Transcript{}, invalid(err)
	}
	t, err := FindTranscript(m.claudeConfigDir(), id)
	if err != nil {
		return Transcript{}, invalid(err)
	}
	m.mu.Lock()
	t.OpenSlot = m.openSlotLocked(id)
	ours := m.pidsLocked()
	m.mu.Unlock()
	if t.OpenSlot == 0 {
		if pid := ExternalPID(m.claudeConfigDir(), id); !ours[pid] {
			t.ExternalPID = pid
		}
	}
	return t, nil
}

// pidsLocked is the set of claude processes ccctl itself runs. m.mu held.
func (m *Manager) pidsLocked() map[int]bool {
	out := map[int]bool{}
	for _, s := range m.slots {
		if s != nil {
			if p := s.Info().PID; p > 0 {
				out[p] = true
			}
		}
	}
	return out
}

func (m *Manager) openSlotLocked(id string) int {
	for i, s := range m.slots {
		if s != nil {
			if inf := s.Info(); inf.SessionID == id && inf.State.Active() {
				return i + 1
			}
		}
	}
	return 0
}
