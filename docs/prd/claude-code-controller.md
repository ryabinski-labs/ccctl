---
spec: spec-clarifier/v1
feature: claude-code-controller
title: Claude Code controller
status: complete
prompt: inline
prompt_sha256: 03f402aafa6836b8e161f1a7d0ac082cbfb123d136356724c0beba7e6f8d70cd
created: 2026-09-30
---

# PRD: Claude Code controller

> Historical design record, written before the project was open-sourced. Where it says the repository is private, or names example hosts and Tailscale addresses, read them as examples. The README is the current user documentation.

## 0. Source prompt

> The risk portfolio portion of the portfolio should be changed for preservation. I want to build a Claude Code controller, which would allow me to run multiple sessions of Claude Code on the remote device see everything it's doing and feed it in input or output. This should be a web interface and should allow me to run four terminals at once working on four different tasks. The connection should be over tail scale.

## 1. Problem

The user runs several Claude Code tasks in parallel on a workstation and has no way to watch or steer them from another device. Today that means SSH plus manual terminal juggling, one session per SSH window, with no signal when a session stops and waits for input. The user wants to supervise up to four Claude Code sessions, each on its own task, from a web browser on any device in their Tailscale tailnet: see every session's live terminal output and type into any of them.

Evidence: the user's request (§0). No existing tool covers this; `tmux` and `ttyd` are installed on neither Mac (checked 2026-09-30).

**Target hosts (inspected over SSH on 2026-09-30):** the first host is `mac-mini` (Tailscale 100.64.0.10, macOS 27.0, arm64, sleep disabled). It has `claude` 2.1.270 at `~/.local/bin/claude`, git 2.50.1, Go, and Node. It has no `tailscale` CLI on PATH: the only Tailscale binary is `/Applications/Tailscale.app/Contents/MacOS/Tailscale`. It has no `~/projects` folder; its git repos sit under `~/Documents` and `~`. Port 7681 is free. The viewing device is `laptop` (100.64.0.20).

## 2. Target user

- **Primary:** the host owner (a single developer) supervising Claude Code sessions on their own macOS or Linux machine from another device on the same tailnet.
- **Secondary:** none in this release.
- **Explicitly not for:** other tailnet members, teammates, or read-only viewers (DL-006); phone-sized screens (DL-005).
- **Job-to-be-done:** When I have four independent coding tasks, I want to start a Claude Code session for each on my workstation and watch and steer all four from one browser page, so I can run them in parallel without sitting at the workstation.

## 3. Outcome and success metric

- **Outcome this moves:** the number of times the user must open SSH or sit at the host to manage a Claude Code session.
- **Primary KPI:** count of days, in the first 14 days after v1 is installed on a host, on which the user had to SSH into the host to start, answer, or stop a Claude Code session. Self-reported (A-001).
- **Baseline:** not instrumented; every remote session today needs SSH.
- **Target:** 0 such days within 14 days of v1 install.
- **Guardrail metrics:** zero lost uncommitted work caused by the controller (worktrees are never removed by it, DL-012); zero requests served to a Tailscale login outside the allowlist (`auth_denied` log line, §9).

## 4. Goals and non-goals

**Goals**
1. Run up to 4 concurrent Claude Code sessions on one host, each in its own pane of a 2x2 browser grid, with live output and full keyboard input (DL-001, DL-010).
2. Reach the controller only over the host's existing Tailscale connection, only as the allowlisted Tailscale login (DL-006, DL-007, DL-009).
3. Isolate each task in its own git worktree and bring every live session back after a controller or host restart (DL-005, DL-013).
4. Tell the user which pane is waiting for input without them reading every pane (DL-005).

**Non-goals**
1. Mobile phone layout and on-screen key bar: the user excluded it from v1 (DL-005).
2. A structured chat or diff view, Agent SDK integration, or `stream-json` rendering: the build is raw terminals (DL-001).
3. Controlling sessions on more than one host from one page: one controller per host (DL-001).
4. tmux integration or attaching to sessions over SSH: sessions live inside the controller process (DL-001).
5. Read-only viewers or multi-user roles: single owner only (DL-006).
6. More than 4 concurrent sessions (DL-010).
7. HTTPS, TLS certificates, and OS-level browser notifications: plain HTTP on the Tailscale IP (DL-009).
8. Per-session permission modes other than bypass (DL-008).
9. Automatic worktree removal (DL-012).
10. The risk-portfolio sentence in §0: it belongs to the day-trading-agent spec, not this one (DL-002).
11. Billing, and storing Claude credentials anywhere but the host's own config: sessions use the host user's existing Claude Code login unless the owner sets session variables such as `CLAUDE_CODE_OAUTH_TOKEN` in the Settings panel (A-007, DL-018).

## 5. User stories and acceptance criteria

Terms: a **slot** is one of the 4 grid panes (1 to 4). A **session** is one `claude` process running in a pseudo-terminal (PTY) owned by the controller. Session states: `starting`, `running`, `needs-input`, `exited`, `resume-failed`. Allowed transitions: `starting` to `running`; `running` to `needs-input` and back; `running` or `needs-input` to `exited`; `starting` (on resume) to `resume-failed`. A slot is free when it has no session or its session is `exited` or `resume-failed`, whether or not the user closed the pane (DL-016). A new launch takes the lowest-numbered free slot and replaces any exited pane in it; the replaced session's worktree and branch are untouched.

Permission matrix (DL-006): the only role is **owner** = a Tailscale login listed in `allowed_logins` (default: the Tailscale login of the host's own node, A-006). The owner may view, launch, type into, stop, close, and resume sessions. Any other caller may do nothing and receives HTTP 403.

- **[P0]** As the owner, I want to launch a Claude Code session into a free slot for a named task in a chosen repo, so that each task runs isolated.
  - [ ] Given a free slot and the launch form filled with task name `fix-login`, repo `~/Documents/folio` picked from the scanned list, worktree on, when I press Start, then within 5 seconds the slot shows a live `claude` TUI whose working directory is the new worktree `~/Documents/folio-wt-fix-login` on new branch `ctl/fix-login` created from the repo's current HEAD, and the process was started with `--dangerously-skip-permissions` and `--session-id` set to a UUID recorded in the state file.
  - [ ] Given the launch form with an optional first prompt of up to 10,000 characters, when I press Start, then `claude` receives that text as its initial prompt.
  - [ ] Given the custom-path field holds `/opt/x` that is not a directory, when I press Start, then the form shows `Path /opt/x does not exist or is not a folder.` and no session starts.
  - [ ] Given the worktree folder `~/Documents/folio-wt-fix-login` already exists or branch `ctl/fix-login` already exists, when I press Start, then the form shows `Worktree folder or branch for fix-login already exists. Choose another task name.` and no session starts.
  - [ ] Given a chosen folder is not a git repository, when the form renders, then the worktree toggle is off and disabled with the note `Not a git repository: session runs in the folder itself.`
  - [ ] Given a task name that is empty, longer than 40 characters, contains characters outside `a-z 0-9 -` after lowercasing, or equals a live session's name, when I press Start, then the form shows `Task name must be 1 to 40 characters of a-z, 0-9 or -, and unique among open sessions.` and no session starts.
  - [ ] Given the configured `claude_path` does not point to an executable file, when I press Start, then the form shows `Claude Code was not found at CLAUDE_PATH. Set claude_path in ~/.ccctl/config.toml.` and no session starts.
- **[P0]** As the owner, I want to see every session's live terminal output and type into any pane, so that I can steer each task.
  - [ ] Given 4 running sessions, when any session writes output, then that pane renders it within 250 milliseconds at p95 over the tailnet, and the other 3 panes keep rendering their own output.
  - [ ] Given I focus pane 2 and type `hello`, Enter, Esc, Ctrl-C, or arrow keys, when the keys are sent, then session 2's PTY receives exactly those bytes and no other session receives them.
  - [ ] Given I click a pane's header maximize button, when the page re-renders, then that pane fills the grid area and the PTY is resized to the new rows and columns; clicking restore returns to 2x2.
- **[P0]** As the owner, I want only my Tailscale identity to reach the controller, so that nobody else gets a shell on my host.
  - [ ] Given a request from a tailnet login not in `allowed_logins`, when it loads the page or opens the WebSocket, then the controller answers HTTP 403 with the page `Not authorized: LOGIN is not on this controller's allowlist.` (LOGIN replaced by the caller's login) and writes an `auth_denied` log line.
  - [ ] Given a WebSocket upgrade whose `Origin` header does not equal the controller's own origin, when it arrives, then the controller rejects it with HTTP 403.
  - [ ] Given the host's LAN or public IP, when a client connects to the controller port on it, then the connection is refused, because the controller listens only on the host's Tailscale IPv4 address.
- **[P0]** As the owner, I want the controller to confirm Tailscale is on and tell me clearly when it is not, so that I know why I cannot connect (DL-007).
  - [ ] Given Tailscale on the host is stopped or logged out at controller start, when the controller starts, then it writes `Tailscale is not running on this host. Start the Tailscale app or run: sudo tailscale up. Retrying every 10 seconds.` to its log and to `ccctl status`, serves nothing, and binds within 10 seconds of Tailscale reporting `Running`.
  - [ ] Given the controller is serving and host Tailscale stops, when the next 10-second check runs, then the controller closes its listener, writes `tailscale_down`, keeps all sessions running, and re-binds when Tailscale returns.
  - [ ] Given an open page whose WebSocket drops, when the drop is detected, then the page shows the banner `Lost connection to HOST. Check that Tailscale is on for this device and for HOST. Retrying in N s.` and retries after 1, 2, 4, 8, 16, then every 30 seconds until it reconnects.
- **[P0]** As the owner, I want at most 4 sessions at once, so that the grid and the host stay within the stated limit (DL-010).
  - [ ] Given 3 slots hold running sessions and slot 4 holds an `exited` pane that was not closed, when I launch a new session, then it starts in slot 4, replacing the exited pane, and the exited session's worktree folder and branch still exist (DL-016).
  - [ ] Given 4 slots hold sessions that are `starting`, `running`, or `needs-input`, when the page renders, then the New session control is disabled with the tooltip `All 4 slots are in use. Stop or close a session first.`, and a direct API launch request returns HTTP 409 with the same text.
- **[P1]** As the owner, I want a pane to flag when its session is waiting for me, so that I do not have to read all four panes.
  - [ ] Given a running session, when Claude Code fires its `Notification` hook or its `Stop` hook, then within 2 seconds that pane's header turns amber with the label `Needs input`, the browser tab title is prefixed with the count of such panes in parentheses, and a single chime plays unless muted in the page header.
  - [ ] Given a pane in `needs-input`, when any keystroke is sent to it, then the label clears and the state returns to `running`.
- **[P1]** As the owner, I want live sessions to come back after the controller or the host restarts, so that a reboot does not lose my four tasks (DL-013).
  - [ ] Given 3 sessions in `running` or `needs-input` when the controller stops, when it starts again, then each is relaunched in its original slot and working directory with `claude --resume SESSION_ID --dangerously-skip-permissions`, before any browser connects.
  - [ ] Given a resumed `claude` exits with a non-zero code within 5 seconds, when this happens, then the pane shows `Could not resume TASK (exit code N).` with buttons Start fresh and Close, and writes a `resume_failed` log line.
  - [ ] Given a pane in `resume-failed` for task TASK, when I press Start fresh, then a new `claude` starts in the same slot, working directory, worktree, and branch under the same task name, with a new session UUID and without `--resume`, and the state file records the new UUID (DL-017).
  - [ ] Given a session the user stopped or that exited on its own before the restart, when the controller starts, then that session is not relaunched.
- **[P1]** As the owner, I want to stop a session and free its slot without losing its work, so that I can start another task (DL-012).
  - [ ] Given a running session, when I press Stop and confirm the dialog `Stop TASK? The worktree and branch are kept.`, then the controller sends SIGHUP to the session's process group, SIGKILL after 5 seconds if it is still alive, and the pane shows `Exited (code N). Worktree kept at PATH on branch ctl/TASK.` with a Close pane button.
  - [ ] Given a session whose `claude` exits on its own, when it exits, then the pane shows the same exited message and the worktree folder and branch still exist.
- **[P1]** As the owner, I want reloading the page or opening it on another device to show what each session already printed, so that I never miss output.
  - [ ] Given a session that printed output while no browser was connected, when a browser connects, then the pane replays up to the last 1 MiB of that session's PTY output before live output continues.
  - [ ] Given two browser tabs open on the controller, when either types into pane 1, then both tabs show the same pane 1 output, and pane 1's PTY size follows the tab that most recently sent input.
- **[P1]** As the owner, I want one command to install the controller as a background service on macOS or Linux, so that it runs whenever the host is up (DL-003, DL-014).
  - [ ] Given a macOS host, when I run `ccctl install`, then it downloads the latest GitHub Release binary for the host's OS and architecture, writes a launchd user agent with `KeepAlive` true, starts it, and `ccctl status` prints the controller URL `http://TAILSCALE_IP:7681`.
  - [ ] Given a Linux host with systemd, when I run `ccctl install`, then it writes and enables a systemd user unit with `Restart=always` and `ccctl status` prints the same URL form.
  - [ ] Given the service process crashes, when launchd or systemd restarts it, then it is serving again within 10 seconds and live sessions are resumed per the resume story.

Priorities: **P0** release fails without it · **P1** ship-blocking unless waived · **P2** follow-up.

## 6. Experience notes

- **Surface:** one single-page web app served by the controller at `http://TAILSCALE_IP:7681/`, plus the `ccctl` CLI on the host (`ccctl serve`, `ccctl install`, `ccctl uninstall`, `ccctl status`, `ccctl hook`). CLI exit codes: 0 success, 1 runtime error, 2 usage error, 3 Tailscale not running (for `status`).
- **Main screen:** a header with the host name, a mute toggle for the chime, and a New session button; below it a 2x2 grid of panes. Each pane header shows slot number, task name, repo folder name, branch, state badge (`Starting`, `Running`, `Needs input`, `Exited`, `Resume failed`), and buttons Maximize and Stop. Panes use xterm.js with the fit add-on.
- **Empty state:** a free slot shows `Slot N is free` and a Start a session button that opens the launch form bound to that slot.
- **Launch form (modal):** task name; repo list (searchable, git repos found up to 3 folder levels under each configured root; default roots `~/projects` and `~/Documents`, missing roots skipped, plus the home folder itself at depth 1 (A-020)); custom path field; worktree toggle (default on for git repos); optional first prompt (multi-line, 10,000 character limit). Start and Cancel.
- **Loading state:** a pane in `starting` shows a spinner and `Starting claude in PATH`.
- **Error states:** the form errors, 403 page, connection banner, and resume-failed pane text are listed verbatim in §5.
- **Platforms:** latest stable desktop Chrome, Safari, and Firefox at viewports of 1280x720 and larger (A-015). Phone layouts are a non-goal.
- **Accessibility bar:** WCAG 2.2 AA for the controller chrome (header, pane headers, form, dialogs, banners): keyboard reachable, visible focus, labelled controls, state badges announced through an `aria-live="polite"` region. Terminal content uses xterm.js with its screen-reader mode available through a header toggle (A-012).

## 7. Scope

**In scope:** the `ccctl` Go binary with embedded Vue 3 + xterm.js frontend; PTY session management for up to 4 `claude` processes; Tailscale state check and Tailscale-IP-only listener; WhoIs allowlist; git worktree creation; needs-input detection through Claude Code hooks; state file and auto-resume; scrollback replay; launchd and systemd install; GitHub Actions CI and release builds.

**Out of scope:** everything in §4 Non-goals.

**Dependencies:** Tailscale installed, logged in, and running on the host and on the viewing device (DL-007); Claude Code CLI installed and logged in on the host; git 2.20 or newer on the host for worktrees; a new private GitHub repository `claude-code-controller` (DL-014).

**Constraints:**
- Go (current stable) backend with `creack/pty`; Vue 3 + Vite + xterm.js frontend embedded with `embed.FS` into one binary (DL-004).
- Runs on macOS (arm64 and amd64) and Linux (arm64 and amd64) (DL-003).
- Uses the host's existing Tailscale daemon through its CLI (located per A-021) or LocalAPI; no embedded tsnet node and no `tailscale serve` (DL-007).
- Plain HTTP on the Tailscale IPv4 address, port 7681 by default, configurable (DL-009, A-005).
- Every session starts with `--dangerously-skip-permissions` (DL-008).
- All changes ship through pull requests and CI; no direct pushes to `main` (user workflow rule, DL-014).

## 8. Risks and open questions

| ID | Risk or question | Type | Owner | Resolve by |
|---|---|---|---|---|
| R-001 | Every session runs in bypass-permissions mode, so any instruction a session follows can run any command as the host user. | Risk | Owner | Accepted by DL-008; mitigated only by the allowlist and the worktree isolation. |
| R-002 | Plain HTTP means page content is visible to software on the viewing device and to any tailnet node that can reach port 7681; WireGuard still encrypts the network path. | Risk | Owner | Accepted by DL-009; mitigated by the WhoIs allowlist on every request and WebSocket upgrade. |
| R-003 | A malicious web page open in the owner's browser could try a cross-site WebSocket to the controller. | Risk | Build | Mitigated by the Origin check in §5. |
| R-004 | A `--resume` of a session whose Claude Code transcript was deleted fails. | Risk | Build | Handled by the resume-failed state in §5. |

## 9. Instrumentation

All events are JSON log lines on the host only; nothing leaves the host (A-013).

| Event | Trigger | Properties | Purpose |
|---|---|---|---|
| session_started | a session reaches `running` | slot, task, cwd, worktree, branch, session_id, resumed | KPI context; audit of what ran |
| session_exited | a `claude` process exits | slot, task, exit_code, duration_s, stopped_by_user | audit |
| needs_input | Notification or Stop hook received | slot, task, hook | alert verification |
| resume_failed | resumed process exits non-zero within 5 s | slot, task, session_id, exit_code | resume reliability |
| auth_denied | request from a login outside the allowlist | login, node, path | guardrail metric |
| tailscale_down | Tailscale check reports not Running | backend_state | connection diagnosis |

## 10. Rollout and rollback

- **Strategy:** full release to the owner's hosts. Each merge to `main` passes CI and publishes a GitHub Release with darwin and linux binaries for arm64 and amd64; the owner runs `ccctl install` (or `ccctl install --version vX.Y.Z`) on each host (DL-014, A-014).
- **Rollback trigger:** any P0 acceptance criterion in §5 failing on a host after an upgrade. Rollback is `ccctl install --version` with the previous release tag, which the CI release job keeps for at least the last 10 releases.
- **Migration or backfill:** none; first release. The state file carries a `version` field starting at 1, and a controller that reads an unknown version refuses to start with `State file version N is newer than this ccctl supports.`

## 11. Compliance gates

- [x] None apply: a single-user developer tool on the owner's own hosts that collects no data about other people, sends nothing to third parties beyond what Claude Code itself already sends, and is not offered to anyone else.

## 12. Data and integrations

- **Stored:** a state file `~/.ccctl/state.json` (mode 0600, written atomically on every state change) holding, per slot: slot number, task name, working directory, worktree path, branch, Claude session UUID, state, launch time. Config file `~/.ccctl/config.toml` holding `roots` (default `["~/projects", "~/Documents"]`, A-020), `claude_path` (A-022), `tailscale_path` (A-021), `port` (default 7681), `allowed_logins` (default: the host node's own login), `max_sessions` fixed at 4, and `env` (variables added to every session; set write-only from the page's Settings panel, DL-018). Terminal output is held only in memory (1 MiB ring buffer per session) and never written to disk by the controller. Logs in `~/.ccctl/logs/`, rotated at 10 MB, 7 files kept (A-019). The only personal data is Tailscale login names in logs.
- **Migration:** none (first release).
- **Integrations:**
  - **Tailscale (host daemon):** the CLI found by A-021, `status --json` every 10 seconds for `BackendState` and the Tailscale IPv4; WhoIs lookup of the caller's address for its login on each HTTP request and WebSocket upgrade (cached per remote address for 60 seconds). Authenticated by local socket access as the host user. Tests replace it with a Go interface fake.
  - **Claude Code CLI:** launched as `claude --dangerously-skip-permissions --session-id UUID --settings FILE [first prompt]` or `claude --resume UUID --dangerously-skip-permissions --settings FILE`. The settings file registers `Notification` and `Stop` hooks that run `ccctl hook --slot N`, which posts to the controller over a Unix socket at `~/.ccctl/ccctl.sock` (A-002). Auth is the host user's existing Claude login (A-007). Tests replace it with a fake `claude` script on PATH that echoes input and invokes the hook command.
  - **git CLI:** `git worktree add -b ctl/TASK ../REPO-wt-TASK HEAD` run in the repo. Tests use temporary repositories.
  - **GitHub Actions and Releases:** CI runs `go vet`, `go test ./...`, the frontend build, and Vitest on every PR; merges to `main` publish releases through GoReleaser (A-018).

## 13. Non-functional requirements

- Controller-added output latency (PTY read to WebSocket write) p95 of 20 milliseconds or less with 4 busy sessions, measured by a Go benchmark on the host (A-011).
- End-to-end output-to-render latency p95 of 250 milliseconds or less between two devices on the same tailnet, measured with a timestamped echo test in CI-independent manual QA.
- Controller resident memory 150 MB or less with 4 sessions and full scrollback buffers, excluding the `claude` processes, measured with `ps` on the host.
- Page first load 2 seconds or less over the tailnet on a desktop browser, measured by the browser performance timeline.
- Binary size 30 MB or less per platform.
- Restart after crash: serving again within 10 seconds under launchd or systemd.
- Availability targets beyond automatic restart: out of scope for this personal tool (A-011).
- Security: WhoIs allowlist check on 100 percent of HTTP requests and WebSocket upgrades; listener bound only to the Tailscale IPv4; state and config files mode 0600.

## 14. Assumptions

| ID | Dimension | Assumption | Why this default | Overturn if |
|---|---|---|---|---|
| A-001 | purpose | Success is 0 days of SSH fallback in the first 14 days after install, self-reported; no telemetry is built for it. | The user asked for a capability outright; a personal tool does not justify metric plumbing. | The user wants a measured usage metric. |
| A-002 | outputs | Needs-input detection uses Claude Code's Notification and Stop hooks injected per session through a generated settings file, calling `ccctl hook` over a Unix socket. | Hooks are the documented signal for "waiting for the user"; screen-scraping the TUI breaks on UI changes. | The hook API changes or the user wants detection without touching Claude settings. |
| A-003 | states | Each session keeps a 1 MiB in-memory ring buffer of raw PTY output replayed on connect; nothing is written to disk. | Covers several screens of Claude TUI history without persisting sensitive output. | The user wants full history across restarts beyond Claude's own `--resume`. |
| A-004 | states | Multiple browser tabs may connect at once; all mirror all panes, all may type, and PTY size follows the tab that most recently sent input. | Simplest model for a single owner moving between devices. | The user wants a single active controller tab. |
| A-005 | errors | Tailscale is checked every 10 seconds; when not Running the controller stops listening, keeps sessions alive, logs the §5 message, and re-binds on recovery. Default port is 7681. | Implements DL-007 without killing work when Tailscale blips. | The user wants the controller to exit when Tailscale is down. |
| A-006 | permissions | `allowed_logins` defaults to the Tailscale login of the host's own node and is editable in config. | Identifies "only me" with zero setup on a personal tailnet. | The host is logged in to Tailscale under a different login from the viewing devices. |
| A-007 | integrations | Sessions use the host user's existing Claude Code login; the controller never handles API keys. | Keeps credentials out of the controller. | The user wants a different Claude account per session. |
| A-008 | inputs | Task names are 1 to 40 characters of a-z, 0-9 or -, unique among open sessions; roots are scanned 3 levels deep; custom paths must be absolute existing folders; the first prompt is capped at 10,000 characters; worktrees branch from the repo's current HEAD. | Task names become folder and branch names, so they must be filesystem and git safe. | The user wants a base-branch picker or free-form names. |
| A-009 | errors | Stop sends SIGHUP to the process group, then SIGKILL after 5 seconds; a resume counts as failed when `claude` exits non-zero within 5 seconds. | Gives Claude Code time to flush its transcript. | Sessions are observed to need longer to shut down. |
| A-010 | data | State and config live under `~/.ccctl/` on both macOS and Linux. | One path to document and back up across both OSes. | The user prefers platform-native config folders. |
| A-011 | non_functional | The §13 numbers are defaults chosen for a single-user tool on a tailnet; availability beyond auto-restart is out of scope. | No existing SLO exists to reuse. | Measured use shows lag or memory pressure. |
| A-012 | interface | WCAG 2.2 AA applies to the controller chrome; terminal content relies on xterm.js screen-reader mode. | Standard bar; terminal emulation cannot meet AA on its own. | The user needs a different accessibility bar. |
| A-013 | rollout | Instrumentation is local JSON log lines only. | Personal tool; nothing should leave the host. | The user wants a dashboard. |
| A-014 | rollout | Releases go to all hosts manually through `ccctl install`; there is no auto-update. | Keeps upgrades in the owner's control and gives a tested rollback path. | The user wants auto-update. |
| A-015 | interface | Supported browsers are the latest stable desktop Chrome, Safari, and Firefox at 1280x720 and larger. | Matches a desktop-only v1 (DL-005). | The user supervises from a tablet. |
| A-016 | outputs | The needs-input alert is in-page only: amber pane header, tab-title count, one chime per transition with a mute toggle. | DL-009 rules out OS notifications on plain HTTP. | The user moves to HTTPS. |
| A-017 | constraints | Each release ships darwin and linux binaries for arm64 and amd64. | Covers Apple Silicon, Intel Macs, and common Linux hosts (DL-003). | A host needs another architecture. |
| A-018 | integrations | CI is GitHub Actions; releases are built with GoReleaser on merge to `main` with the version computed by the workflow. | Common Go release path and fits the PR-and-CI rule. | The user has an existing release tool to reuse. |
| A-020 | inputs | Default scan roots are `~/projects` and `~/Documents` (missing ones skipped) plus direct child folders of the home folder. | mac-mini keeps 9 repos under `~/Documents` and 2 under `~`; the MacBook uses `~/projects`. | The user wants a single explicit root. |
| A-021 | integrations | The Tailscale CLI is located as: `tailscale_path` from config, else `tailscale` on PATH, else `/Applications/Tailscale.app/Contents/MacOS/Tailscale`. If none exists, startup logs `Tailscale CLI not found. Install Tailscale or set tailscale_path in ~/.ccctl/config.toml.` and retries every 10 seconds. | mac-mini has only the app-bundle binary, no `tailscale` on PATH. | Tailscale ships a different path. |
| A-022 | integrations | `ccctl install` records the absolute path of `claude` from the installing shell into `claude_path`, because launchd and systemd services do not load the user's shell PATH. | On mac-mini `claude` lives in `~/.local/bin`, which a launchd agent's PATH does not include. | Claude Code moves or is reinstalled elsewhere. |
| A-019 | data | Logs rotate at 10 MB and keep 7 files; the controller never writes terminal output to disk. | Bounded disk use; terminal output can hold secrets. | The user wants an output archive. |

## 15. Decision log

| ID | Dimension | Question | Options offered | Answer | Applied in |
|---|---|---|---|---|---|
| DL-001 | purpose | Which build do you mean by running multiple Claude Code sessions in four terminals at once? | A. Live terminals: real claude TUIs in PTYs streamed to a 2x2 browser grid (recommended) B. Live terminals plus tmux C. Structured chat view via Agent SDK or stream-json D. Multi-host fleet | A | §1, §4, §7 |
| DL-002 | non_goals | What should happen to the stray risk-portfolio sentence from the day-trading-agent prompt? | A. Ignore it here (recommended) B. Apply to the trading spec later C. It belongs here | A | §4 |
| DL-003 | constraints | Which machine is the remote device that runs the sessions? | A. macOS plus Linux (recommended) B. This Mac only C. Linux server only | A | §7, §5 |
| DL-004 | constraints | What stack should the controller be built in? | A. Go plus Vue, one binary (recommended) B. Node or TypeScript C. Rust plus Vue | A | §7 |
| DL-005 | non_goals | Which neighbors are in v1? | A. Needs-input alert B. Mobile phone layout C. Worktree per task D. Survive host restart | A, C, D (mobile layout is a non-goal) | §4, §5 |
| DL-006 | permissions | Who may open the controller and type into sessions? | A. Only me via Tailscale WhoIs allowlist (recommended) B. Anyone on my tailnet C. Me plus read-only viewers | A | §2, §5 |
| DL-007 | constraints | How should the controller be reached over Tailscale? | A. Embedded tsnet node (recommended) B. Bind the host's 100.x IP C. localhost plus tailscale serve | Custom: Tailscale is already installed on both machines; make sure it is on and give an error message if it is not on. | §5, §7, A-005 |
| DL-008 | inputs | Which permission mode should new Claude sessions launch in? | A. Pick per session (recommended) B. Always default C. Always bypass | C | §5, §7, §8 |
| DL-009 | interface | Using the host's existing Tailscale, how should the controller serve the page? | A. HTTPS via tailscale cert (recommended) B. Plain HTTP on the 100.x IP | B | §4, §6, §7 |
| DL-010 | states | Is four sessions a hard limit? | A. Hard cap of 4 (recommended) B. Configurable cap with 4 default C. 4 visible, more in background | A | §4, §5 |
| DL-011 | inputs | When you start a session, how do you choose the repo it works in? | A. Pick from scanned roots (recommended) B. Free-text path C. Both | C | §5, §6 |
| DL-012 | states | When a task's session ends, what happens to its git worktree? | A. Keep it (recommended) B. Remove if clean C. Remove button, never auto | A | §4, §5 |
| DL-013 | states | After the controller or host restarts, how do the previous sessions come back? | A. Auto-resume in place (recommended) B. Resume button per pane | A | §5 |
| DL-014 | rollout | Where does the code live, and how does it get onto the hosts? | A. Private GitHub repo plus CI with release binaries and ccctl install (recommended) B. Local only | A | §7, §10 |
| DL-015 | acceptance | Which stories must work for v1 to count as shipped? | A. As proposed: launch, watch and type, only-me gate, Tailscale check, cap of 4 are P0; the rest P1 (recommended) B. Everything P0 C. Also promote needs-input alert and auto-resume | A | §5 |
| DL-016 | states | With 3 sessions running and slot 4 holding an exited pane not yet closed, what should New session do? | A. Reuse the exited pane (recommended) B. Must close it first | A | §5 |
| DL-017 | states | After a resume fails, what does Start fresh launch? | A. New session with a new UUID in the same slot, worktree, and branch (recommended) B. Open the launch form pre-filled | A | §5 |
| DL-018 | integrations | Where should the owner set session environment variables such as CLAUDE_CODE_OAUTH_TOKEN and GEMINI_API_KEY? | A. Settings panel in the page (recommended) B. Config file only C. ccctl env command D. Per-session field | A: a write-only Settings panel storing `[env]` in `~/.ccctl/config.toml` (0600), applied to sessions launched afterwards; supersedes non-goal 11 in part | §4, §12 |

## Coverage

| Dimension | Status | Ref |
|---|---|---|
| purpose | assumed | §1, §3, A-001 |
| actors | specified | §2, DL-006 |
| permissions | specified | §5, A-006 |
| triggers | specified | §5, §6 |
| inputs | specified | §5, DL-011, A-008, A-020 |
| outputs | specified | §5, A-002, A-016 |
| states | specified | §5, A-003, A-004 |
| errors | specified | §5, A-005, A-009 |
| interface | specified | §6, A-012, A-015 |
| data | specified | §12, A-010, A-019 |
| integrations | specified | §12, A-007, A-018, A-021, A-022 |
| non_functional | assumed | §13, A-011 |
| constraints | specified | §7, A-017 |
| non_goals | specified | §4, DL-002, DL-005 |
| acceptance | specified | §5, DL-015 |
| rollout | specified | §9, §10, A-013, A-014 |
