package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/ryabinski-labs/ccctl/internal/config"
)

// Spec messages (§5), verbatim.
const (
	MsgTaskName       = "Task name must be 1 to 40 characters of a-z, 0-9 or -, and unique among open sessions."
	MsgNotGitRepo     = "Not a git repository: session runs in the folder itself."
	MaxPromptLen      = 10000
	MsgBadAgent       = "Agent must be claude or opencode."
	MsgOpenCodeResume = "Resume by ID is Claude Code only in this release."
)

// Agent is the coding agent that runs in a slot.
type Agent string

const (
	AgentClaude   Agent = "claude"
	AgentOpenCode Agent = "opencode"
)

// NormalizeAgent maps an empty agent to Claude (the pre-existing behavior) and
// rejects any other value.
func NormalizeAgent(s string) (Agent, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", string(AgentClaude):
		return AgentClaude, nil
	case string(AgentOpenCode):
		return AgentOpenCode, nil
	default:
		return "", errors.New(MsgBadAgent)
	}
}

// Valid reports whether a is one of the two supported agents.
func (a Agent) Valid() bool { return a == AgentClaude || a == AgentOpenCode }

// MsgAllInUse is the launch refusal when every slot is active.
var MsgAllInUse = fmt.Sprintf("All %d slots are in use. Stop or close a session first.", config.MaxSessions)

func MsgBadPath(p string) string { return fmt.Sprintf("Path %s does not exist or is not a folder.", p) }
func MsgWorktreeExists(task string) string {
	return fmt.Sprintf("Worktree folder or branch for %s already exists. Choose another task name.", task)
}
func MsgClaudeNotFound(p string) string {
	return fmt.Sprintf("Claude Code was not found at %s. Set claude_path in ~/.ccctl/config.toml.", p)
}

func MsgOpenCodeNotFound(p string) string {
	return fmt.Sprintf("OpenCode was not found at %s. Set opencode_path in ~/.ccctl/config.toml.", p)
}
func MsgExited(code int, worktree, branch, cwd string) string {
	if worktree == "" {
		return fmt.Sprintf("Exited (code %d).", code)
	}
	return fmt.Sprintf("Exited (code %d). Worktree kept at %s on branch %s.", code, worktree, branch)
}
func MsgResumeFailed(task string, code int) string {
	return fmt.Sprintf("Could not resume %s (exit code %d).", task, code)
}

var taskRe = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

// ValidateTask lowercases name and checks the A-008 rules against open task names.
func ValidateTask(name string, open []string) (string, error) {
	n := strings.ToLower(name)
	if !taskRe.MatchString(n) {
		return "", errors.New(MsgTaskName)
	}
	for _, o := range open {
		if o == n {
			return "", errors.New(MsgTaskName)
		}
	}
	return n, nil
}

// ValidatePath requires an absolute path to an existing folder.
func ValidatePath(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", errors.New(MsgBadPath(p))
	}
	fi, err := os.Stat(p)
	if err != nil || !fi.IsDir() {
		return "", errors.New(MsgBadPath(p))
	}
	return filepath.Clean(p), nil
}

// ValidatePrompt caps the first prompt at 10,000 characters.
func ValidatePrompt(p string) error {
	if len([]rune(p)) > MaxPromptLen {
		return fmt.Errorf("First prompt must be at most %d characters.", MaxPromptLen)
	}
	return nil
}

// CheckClaude verifies claude_path is an executable file.
func CheckClaude(p string) error {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
		return errors.New(MsgClaudeNotFound(p))
	}
	return nil
}

// CheckAgent verifies the executable for the selected agent. Both agents are
// checked in the same way; only the message differs.
func CheckAgent(agent Agent, claudePath, openCodePath string) error {
	if agent == AgentOpenCode {
		fi, err := os.Stat(openCodePath)
		if err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
			return errors.New(MsgOpenCodeNotFound(openCodePath))
		}
		return nil
	}
	return CheckClaude(claudePath)
}

// OpenCodeSessionPrefix is required by OpenCode: its session IDs must start
// with "ses". Claude Code uses a UUID.
const OpenCodeSessionPrefix = "ses_"

// NewSessionID returns a fresh session ID for agent.
func NewSessionID(agent Agent) string {
	if agent == AgentOpenCode {
		return OpenCodeSessionPrefix + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	return uuid.NewString()
}

// ValidSessionID reports whether id is usable for agent. Claude Code accepts the
// UUIDs ccctl has always generated; OpenCode requires the "ses" prefix.
func ValidSessionID(agent Agent, id string) bool {
	return agent != AgentOpenCode || strings.HasPrefix(id, OpenCodeSessionPrefix)
}

// FreshArgv builds the Claude launch argv (§12). A prompt starting with "-" is
// preceded by "--".
func FreshArgv(claude, sessionID, settings, prompt string) []string {
	a := []string{claude, "--dangerously-skip-permissions", "--session-id", sessionID, "--settings", settings}
	if prompt != "" {
		if strings.HasPrefix(prompt, "-") {
			a = append(a, "--")
		}
		a = append(a, prompt)
	}
	return a
}

// ResumeArgv builds the Claude resume argv (§5 resume story).
func ResumeArgv(claude, sessionID, settings string) []string {
	return []string{claude, "--resume", sessionID, "--dangerously-skip-permissions", "--settings", settings}
}

// OpenCodeFreshArgv builds a fresh OpenCode TUI argv. --session creates the
// session when the ID does not exist yet, so ccctl owns the ID it persists.
func OpenCodeFreshArgv(opencode, sessionID, prompt string) []string {
	a := []string{opencode, "--session", sessionID, "--auto"}
	if prompt != "" {
		a = append(a, "--prompt", prompt)
	}
	return a
}

// OpenCodeResumeArgv continues an existing OpenCode session with auto-approved
// permissions. No prompt is passed on resume.
func OpenCodeResumeArgv(opencode, sessionID string) []string {
	return []string{opencode, "--session", sessionID, "--auto"}
}

// AgentFreshArgv dispatches the fresh-launch argv by agent.
func AgentFreshArgv(agent Agent, claude, opencode, sessionID, settings, prompt string) []string {
	if agent == AgentOpenCode {
		return OpenCodeFreshArgv(opencode, sessionID, prompt)
	}
	return FreshArgv(claude, sessionID, settings, prompt)
}

// AgentResumeArgv dispatches the resume argv by agent.
func AgentResumeArgv(agent Agent, claude, opencode, sessionID, settings string) []string {
	if agent == AgentOpenCode {
		return OpenCodeResumeArgv(opencode, sessionID)
	}
	return ResumeArgv(claude, sessionID, settings)
}

// HookSettings renders the per-session settings file registering Notification
// and Stop hooks that call `ccctl hook --slot N` (A-002).
func HookSettings(ccctl string, slot int) ([]byte, error) {
	cmd := fmt.Sprintf("%s hook --slot %d", ccctl, slot)
	entry := []map[string]any{{"matcher": "", "hooks": []map[string]any{{"type": "command", "command": cmd}}}}
	return json.MarshalIndent(map[string]any{"hooks": map[string]any{"Notification": entry, "Stop": entry}}, "", "  ")
}

// State of a session (§5).
type State string

const (
	Starting     State = "starting"
	Running      State = "running"
	NeedsInput   State = "needs-input"
	Exited       State = "exited"
	ResumeFailed State = "resume-failed"
)

// Active reports whether the state occupies a slot.
func (s State) Active() bool { return s == Starting || s == Running || s == NeedsInput }

// NextSlot returns the lowest free slot (1-based) or 0 when all are active (DL-016).
// states[i] == "" means the slot is empty.
func NextSlot(states []State) int {
	for i, s := range states {
		if !s.Active() {
			return i + 1
		}
	}
	return 0
}
