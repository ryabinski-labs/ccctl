package session

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"

	"github.com/ryabinski-labs/ccctl/internal/config"
	"github.com/ryabinski-labs/ccctl/internal/logx"
	"github.com/ryabinski-labs/ccctl/internal/state"
)

// Info is the public view of a slot (protocol SlotInfo).
type Info struct {
	Slot       int       `json:"slot"`
	Task       string    `json:"task"`
	Cwd        string    `json:"cwd"`
	Repo       string    `json:"repo"`
	Worktree   string    `json:"worktree"`
	Branch     string    `json:"branch"`
	SessionID  string    `json:"session_id"`
	State      State     `json:"state"`
	LaunchedAt time.Time `json:"launched_at"`
	Resumed    bool      `json:"resumed"`
	ExitCode   *int      `json:"exit_code"`
	Message    string    `json:"message"`
	PID        int       `json:"-"`
}

// Errors carrying HTTP semantics.
var (
	ErrFull     = errors.New(MsgAllInUse)
	ErrNotFound = errors.New("No session in that slot.")
)

// ValidationError is a refused launch (HTTP 400) with the spec text.
type ValidationError struct{ Msg string }

func (e ValidationError) Error() string { return e.Msg }

func invalid(err error) error { return ValidationError{err.Error()} }

type Options struct {
	Home        string // expands ~ in launch paths
	ClaudePath  string
	CCCTLPath   string
	SettingsDir string
	SockPath    string
	Env         []string
	Store       *state.Store
	Log         *slog.Logger
	StopGrace   time.Duration
	ResumeGrace time.Duration
	// OnChange is called (without locks held) whenever a slot's info changes; nil info = slot cleared.
	OnChange func(slot int, info *Info)
	// SessionEnv returns extra variables for each new session (config [env]); may be nil.
	SessionEnv func() map[string]string
}

type Manager struct {
	opt     Options
	mu      sync.Mutex
	slots   [config.MaxSessions]*Session
	closing bool
}

func NewManager(o Options) *Manager {
	if o.StopGrace == 0 {
		o.StopGrace = 5 * time.Second
	}
	if o.ResumeGrace == 0 {
		o.ResumeGrace = 5 * time.Second
	}
	if o.Log == nil {
		o.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if o.OnChange == nil {
		o.OnChange = func(int, *Info) {}
	}
	return &Manager{opt: o}
}

// SetClaudePath updates the configured claude binary.
func (m *Manager) SetClaudePath(p string) { m.mu.Lock(); m.opt.ClaudePath = p; m.mu.Unlock() }

// Session is one claude process in a PTY.
type Session struct {
	m         *Manager
	mu        sync.Mutex
	info      Info
	ptmx      *os.File
	cmd       *exec.Cmd
	ring      *Ring
	subs      map[*Sub]struct{}
	done      chan struct{}
	stopped   bool
	started   time.Time
	owner     string
	sizes     map[string][2]uint16
	curSize   [2]uint16
	writeMu   sync.Mutex
	ptyClosed bool // set under mu once ptmx is closed
}

// Sub receives a session's output. C is closed when the subscriber is dropped
// (too slow) or the session goes away.
type Sub struct {
	C      chan []byte
	closed bool
}

// Slots returns a copy of every slot's info (nil = empty).
func (m *Manager) Slots() []*Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Info, config.MaxSessions)
	for i, s := range m.slots {
		if s != nil {
			inf := s.Info()
			out[i] = &inf
		}
	}
	return out
}

func (s *Session) Info() Info { s.mu.Lock(); defer s.mu.Unlock(); return s.info }

func (m *Manager) session(slot int) (*Session, error) {
	if slot < 1 || slot > config.MaxSessions {
		return nil, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.slots[slot-1]; s != nil {
		return s, nil
	}
	return nil, ErrNotFound
}

// LaunchRequest is POST /api/sessions.
type LaunchRequest struct {
	Task     string `json:"task"`
	Path     string `json:"path"`
	Worktree bool   `json:"worktree"`
	Prompt   string `json:"prompt"`
	// Slot is the pane the user clicked; honored when free, else the lowest free slot is used.
	Slot int `json:"slot"`
	// ResumeID resumes a saved claude session (an ID or a pasted `claude --resume` command)
	// in its own folder instead of starting fresh in Path.
	ResumeID string `json:"resume_id"`
}

// Launch validates req and starts a fresh session in the lowest free slot.
func (m *Manager) Launch(req LaunchRequest) (Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing {
		return Info{}, errors.New("Controller is shutting down.")
	}
	states := make([]State, config.MaxSessions)
	var open []string
	for i, s := range m.slots {
		if s != nil {
			inf := s.Info()
			states[i] = inf.State
			if inf.State.Active() {
				open = append(open, inf.Task)
			}
		}
	}
	slot := NextSlot(states)
	if slot == 0 {
		return Info{}, ErrFull
	}
	if req.Slot >= 1 && req.Slot <= config.MaxSessions && !states[req.Slot-1].Active() {
		slot = req.Slot
	}
	task, err := ValidateTask(req.Task, open)
	if err != nil {
		return Info{}, invalid(err)
	}
	if strings.TrimSpace(req.ResumeID) != "" {
		return m.resumeByIDLocked(slot, task, req)
	}
	dir, err := ValidatePath(config.Expand(m.opt.Home, strings.TrimSpace(req.Path)))
	if err != nil {
		return Info{}, invalid(err)
	}
	if err := ValidatePrompt(req.Prompt); err != nil {
		return Info{}, invalid(err)
	}
	if err := CheckClaude(m.opt.ClaudePath); err != nil {
		return Info{}, invalid(err)
	}
	info := Info{Slot: slot, Task: task, Cwd: dir, Repo: filepath.Base(dir)}
	if req.Worktree && IsGitRepo(dir) {
		plan, err := PlanWorktree(dir, task)
		if err != nil {
			return Info{}, invalid(err)
		}
		if err := plan.Check(task); err != nil {
			return Info{}, invalid(err)
		}
		if err := plan.Create(); err != nil {
			return Info{}, invalid(err)
		}
		info.Repo, info.Worktree, info.Branch, info.Cwd = filepath.Base(plan.Top), plan.Path, plan.Branch, plan.Cwd
	}
	return m.startLocked(info, req.Prompt, false)
}

// resumeByIDLocked starts `claude --resume <id>` in the folder the session was recorded in. m.mu held.
func (m *Manager) resumeByIDLocked(slot int, task string, req LaunchRequest) (Info, error) {
	if req.Prompt != "" || req.Worktree {
		return Info{}, ValidationError{MsgResumeWithInput}
	}
	id, err := ParseSessionID(req.ResumeID)
	if err != nil {
		return Info{}, invalid(err)
	}
	if n := m.openSlotLocked(id); n != 0 {
		return Info{}, ValidationError{MsgAlreadyOpen(id, n)}
	}
	t, err := FindTranscript(m.claudeConfigDir(), id)
	if err != nil {
		return Info{}, invalid(err)
	}
	dir, err := ValidatePath(t.Cwd)
	if err != nil {
		return Info{}, ValidationError{MsgTranscriptFolderGone(t.Cwd)}
	}
	if err := CheckClaude(m.opt.ClaudePath); err != nil {
		return Info{}, invalid(err)
	}
	return m.startLocked(Info{Slot: slot, Task: task, Cwd: dir, Repo: filepath.Base(dir), SessionID: id}, "", true)
}

// startLocked spawns claude for info in info.Slot, replacing whatever was there. m.mu held.
func (m *Manager) startLocked(info Info, prompt string, resume bool) (Info, error) {
	if !resume {
		info.SessionID = uuid.NewString()
	}
	info.State, info.Resumed, info.LaunchedAt, info.ExitCode, info.Message = Starting, resume, time.Now().UTC(), nil, ""
	settings, err := m.writeSettings(info.Slot, info.SessionID)
	if err != nil {
		return Info{}, err
	}
	var argv []string
	if resume {
		argv = ResumeArgv(m.opt.ClaudePath, info.SessionID, settings)
	} else {
		argv = FreshArgv(m.opt.ClaudePath, info.SessionID, settings, prompt)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = info.Cwd
	cmd.Env = m.env(info.Slot)
	s := &Session{m: m, info: info, cmd: cmd, ring: NewRing(RingSize), subs: map[*Sub]struct{}{},
		done: make(chan struct{}), sizes: map[string][2]uint16{}, curSize: [2]uint16{40, 120}, started: time.Now()}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if old := m.slots[info.Slot-1]; old != nil {
		old.dropSubs()
	}
	m.slots[info.Slot-1] = s
	if err != nil {
		code := -1
		s.info.ExitCode = &code
		if resume {
			s.info.State, s.info.Message = ResumeFailed, MsgResumeFailed(info.Task, code)
		} else {
			s.info.State, s.info.Message = Exited, MsgExited(code, info.Worktree, info.Branch, info.Cwd)
		}
		close(s.done)
		m.persistLocked()
		go m.opt.OnChange(info.Slot, &s.info)
		return s.info, nil
	}
	s.ptmx = ptmx
	s.info.PID = cmd.Process.Pid
	m.persistLocked()
	inf := s.info
	go m.opt.OnChange(info.Slot, &inf)
	go s.readLoop()
	if resume {
		time.AfterFunc(m.opt.ResumeGrace, func() { s.markRunning() })
	}
	return inf, nil
}

// inheritedMarkers are per-session variables a parent Claude Code session sets;
// passing them on makes the child think it is a subagent (no transcript, so no
// --resume). User configuration variables such as CLAUDE_CODE_USE_BEDROCK pass through.
var inheritedMarkers = map[string]bool{
	"CLAUDECODE": true, "CLAUDE_CODE_ENTRYPOINT": true, "CLAUDE_CODE_CHILD_SESSION": true,
	"CLAUDE_CODE_SESSION_ID": true, "CLAUDE_CODE_SESSION_ATTENDED": true, "CLAUDE_CODE_MESSAGING_SOCKET": true,
	"CLAUDE_CODE_MESSAGING_TOKEN": true, "CLAUDE_CODE_EXECPATH": true, "CLAUDE_PID": true,
	"CLAUDE_CODE_STOP_HOOK_BLOCK_CAP": true, "CLAUDE_CODE_SSE_PORT": true,
}

func (m *Manager) env(slot int) []string {
	var env []string
	for _, e := range m.opt.Env {
		if k, _, _ := strings.Cut(e, "="); inheritedMarkers[k] || k == "TERM" || strings.HasPrefix(k, "CCCTL_") {
			continue
		}
		env = append(env, e)
	}
	if m.opt.SessionEnv != nil {
		for k, v := range m.opt.SessionEnv() {
			if config.ValidEnvName(k) && !config.ReservedEnv(k) {
				env = append(env, k+"="+v)
			}
		}
	}
	return append(env, "TERM=xterm-256color", "COLORTERM=truecolor",
		"CCCTL_SOCK="+m.opt.SockPath, fmt.Sprintf("CCCTL_SLOT=%d", slot))
}

func (m *Manager) writeSettings(slot int, id string) (string, error) {
	b, err := HookSettings(m.opt.CCCTLPath, slot)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(m.opt.SettingsDir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(m.opt.SettingsDir, fmt.Sprintf("slot-%d-%s.json", slot, id))
	return p, os.WriteFile(p, b, 0o600)
}

func (m *Manager) persistLocked() {
	if m.opt.Store == nil {
		return
	}
	var out []state.Slot
	for _, s := range m.slots {
		if s == nil {
			continue
		}
		i := s.Info()
		out = append(out, state.Slot{Slot: i.Slot, Task: i.Task, Cwd: i.Cwd, Repo: i.Repo, Worktree: i.Worktree,
			Branch: i.Branch, SessionID: i.SessionID, State: string(i.State), LaunchedAt: i.LaunchedAt})
	}
	if err := m.opt.Store.Save(out); err != nil {
		m.opt.Log.Error("state_save_failed", "error", err.Error())
	}
}

func (m *Manager) persist() { m.mu.Lock(); defer m.mu.Unlock(); m.persistLocked() }

func (s *Session) changed() {
	inf := s.Info()
	s.m.opt.OnChange(inf.Slot, &inf)
}

func (s *Session) markRunning() {
	s.mu.Lock()
	if s.info.State != Starting {
		s.mu.Unlock()
		return
	}
	s.info.State = Running
	i := s.info
	s.mu.Unlock()
	logx.Event(s.m.opt.Log, "session_started", "slot", i.Slot, "task", i.Task, "cwd", i.Cwd, "worktree", i.Worktree,
		"branch", i.Branch, "session_id", i.SessionID, "resumed", i.Resumed)
	s.m.persist()
	s.changed()
}

func (s *Session) readLoop() {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			s.mu.Lock()
			s.ring.Write(chunk)
			for sub := range s.subs {
				select {
				case sub.C <- chunk:
				default: // too slow: drop; the client reconnects and replays.
					delete(s.subs, sub)
					sub.closed = true
					close(sub.C)
				}
			}
			first := s.info.State == Starting && !s.info.Resumed
			s.mu.Unlock()
			if first {
				s.markRunning()
			}
		}
		if err != nil {
			break
		}
	}
	s.wait()
}

func (s *Session) wait() {
	err := s.cmd.Wait()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
			if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				code = 128 + int(ws.Signal())
			}
		} else {
			code = -1
		}
	}
	s.mu.Lock()
	s.ptmx.Close()
	s.ptyClosed = true
	s.mu.Unlock()
	s.m.mu.Lock()
	closing := s.m.closing
	current := s.m.slots[s.info.Slot-1] == s
	s.m.mu.Unlock()
	s.mu.Lock()
	close(s.done)
	if closing || !current {
		s.mu.Unlock()
		s.dropSubs()
		return
	}
	i := &s.info
	i.ExitCode = &code
	dur := time.Since(s.started)
	if i.Resumed && code != 0 && dur < s.m.opt.ResumeGrace && !s.stopped {
		i.State, i.Message = ResumeFailed, MsgResumeFailed(i.Task, code)
		logx.Event(s.m.opt.Log, "resume_failed", "slot", i.Slot, "task", i.Task, "session_id", i.SessionID, "exit_code", code)
	} else {
		i.State, i.Message = Exited, MsgExited(code, i.Worktree, i.Branch, i.Cwd)
	}
	logx.Event(s.m.opt.Log, "session_exited", "slot", i.Slot, "task", i.Task, "exit_code", code,
		"duration_s", int(dur.Seconds()), "stopped_by_user", s.stopped)
	s.mu.Unlock()
	s.m.persist()
	s.changed()
}

func (s *Session) dropSubs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for sub := range s.subs {
		delete(s.subs, sub)
		if !sub.closed {
			sub.closed = true
			close(sub.C)
		}
	}
}

// Subscribe returns the scrollback snapshot and a live channel, atomically so
// nothing is lost or duplicated between them (REQ-011).
func (m *Manager) Subscribe(slot int) ([]byte, *Sub, func(), error) {
	s, err := m.session(slot)
	if err != nil {
		return nil, nil, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sub := &Sub{C: make(chan []byte, 1024)}
	snap := s.ring.Snapshot()
	select {
	case <-s.done:
		sub.closed = true
		close(sub.C)
	default:
		s.subs[sub] = struct{}{}
	}
	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.subs[sub]; ok {
			delete(s.subs, sub)
			sub.closed = true
			close(sub.C)
		}
	}
	return snap, sub, cancel, nil
}

// Input writes bytes to the slot's PTY; the sender becomes the size owner and
// needs-input clears (REQ-008, REQ-011).
func (m *Manager) Input(slot int, client string, data []byte) error {
	s, err := m.session(slot)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.ptmx == nil || isClosed(s.done) {
		s.mu.Unlock()
		return nil
	}
	s.owner = client
	if sz, ok := s.sizes[client]; ok {
		s.applySizeLocked(sz)
	}
	cleared := s.info.State == NeedsInput && !IsTerminalReply(data)
	if cleared {
		s.info.State = Running
	}
	s.mu.Unlock()
	if cleared {
		m.persist()
		s.changed()
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = s.ptmx.Write(data)
	return err
}

// Resize records client's size for slot and applies it if client owns the size or nobody does.
func (m *Manager) Resize(slot int, client string, rows, cols uint16) error {
	if rows == 0 || cols == 0 {
		return nil
	}
	s, err := m.session(slot)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sizes[client] = [2]uint16{rows, cols}
	if s.owner == "" || s.owner == client {
		s.applySizeLocked(s.sizes[client])
	}
	return nil
}

func (s *Session) applySizeLocked(sz [2]uint16) {
	if s.ptmx == nil || s.ptyClosed || sz == s.curSize {
		return
	}
	if err := pty.Setsize(s.ptmx, &pty.Winsize{Rows: sz[0], Cols: sz[1]}); err == nil {
		s.curSize = sz
	}
}

// Forget removes a disconnected client's size entries.
func (m *Manager) Forget(client string) {
	m.mu.Lock()
	ss := m.slots
	m.mu.Unlock()
	for _, s := range ss {
		if s != nil {
			s.mu.Lock()
			delete(s.sizes, client)
			if s.owner == client {
				s.owner = ""
			}
			s.mu.Unlock()
		}
	}
}

// PTYSize returns the slot's current PTY rows and cols.
func (m *Manager) PTYSize(slot int) (uint16, uint16, error) {
	s, err := m.session(slot)
	if err != nil {
		return 0, 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptmx == nil || s.ptyClosed {
		return 0, 0, ErrNotFound
	}
	r, c, err := pty.Getsize(s.ptmx)
	return uint16(r), uint16(c), err
}

// Hook handles a Notification or Stop hook for slot (REQ-008).
func (m *Manager) Hook(slot int, hook, sessionID string) error {
	s, err := m.session(slot)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if sessionID != "" && sessionID != s.info.SessionID {
		s.mu.Unlock()
		return nil
	}
	i := s.info
	flip := i.State == Running || i.State == Starting
	if flip {
		s.info.State = NeedsInput
	}
	s.mu.Unlock()
	logx.Event(m.opt.Log, "needs_input", "slot", slot, "task", i.Task, "hook", hook)
	if flip {
		m.persist()
		s.changed()
	}
	return nil
}

// Stop sends SIGHUP to the process group, then SIGKILL after StopGrace (REQ-010).
func (m *Manager) Stop(slot int) error {
	s, err := m.session(slot)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if isClosed(s.done) || s.cmd.Process == nil {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	pid := s.cmd.Process.Pid
	s.mu.Unlock()
	syscall.Kill(-pid, syscall.SIGHUP)
	go func() {
		select {
		case <-s.done:
		case <-time.After(m.opt.StopGrace):
			syscall.Kill(-pid, syscall.SIGKILL)
		}
	}()
	return nil
}

// Close clears an exited or resume-failed pane.
func (m *Manager) Close(slot int) error {
	s, err := m.session(slot)
	if err != nil {
		return err
	}
	if s.Info().State.Active() {
		return ValidationError{"Stop the session before closing its pane."}
	}
	m.mu.Lock()
	m.slots[slot-1] = nil
	m.persistLocked()
	m.mu.Unlock()
	s.dropSubs()
	m.opt.OnChange(slot, nil)
	return nil
}

// Fresh starts a new session (new UUID, no --resume) in a resume-failed slot's folder (DL-017).
func (m *Manager) Fresh(slot int) (Info, error) {
	s, err := m.session(slot)
	if err != nil {
		return Info{}, err
	}
	i := s.Info()
	if i.State != ResumeFailed {
		return Info{}, ValidationError{"Start fresh is only available after a failed resume."}
	}
	if err := CheckClaude(m.opt.ClaudePath); err != nil {
		return Info{}, invalid(err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startLocked(Info{Slot: i.Slot, Task: i.Task, Cwd: i.Cwd, Repo: i.Repo, Worktree: i.Worktree, Branch: i.Branch}, "", false)
}

// Resume relaunches every saved active session with --resume (REQ-009).
func (m *Manager) Resume() error {
	if m.opt.Store == nil {
		return nil
	}
	f, err := m.opt.Store.Load()
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sl := range f.Slots {
		if !State(sl.State).Active() || sl.Slot < 1 || sl.Slot > config.MaxSessions {
			continue
		}
		info := Info{Slot: sl.Slot, Task: sl.Task, Cwd: sl.Cwd, Repo: sl.Repo, Worktree: sl.Worktree, Branch: sl.Branch, SessionID: sl.SessionID}
		if _, err := m.startLocked(info, "", true); err != nil {
			m.opt.Log.Error("resume_spawn_failed", "slot", sl.Slot, "error", err.Error())
		}
	}
	m.persistLocked()
	return nil
}

// Shutdown hangs up every session without recording them as exited, so they
// resume on the next start.
func (m *Manager) Shutdown(timeout time.Duration) {
	m.mu.Lock()
	m.closing = true
	ss := m.slots
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, s := range ss {
		if s == nil || s.cmd.Process == nil || isClosed(s.done) {
			continue
		}
		wg.Add(1)
		go func(s *Session) {
			defer wg.Done()
			pid := s.cmd.Process.Pid
			syscall.Kill(-pid, syscall.SIGHUP)
			select {
			case <-s.done:
			case <-time.After(timeout):
				syscall.Kill(-pid, syscall.SIGKILL)
				<-s.done
			}
		}(s)
	}
	wg.Wait()
}

// PID returns the slot's process id (0 if none).
func (m *Manager) PID(slot int) int {
	s, err := m.session(slot)
	if err != nil {
		return 0
	}
	return s.Info().PID
}

func isClosed(c chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}
