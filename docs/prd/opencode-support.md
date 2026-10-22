---
spec: spec-clarifier/v1
feature: opencode-support
title: OpenCode sessions alongside Claude Code
status: complete
prompt: inline
prompt_sha256: e4a55c292f3e6d2496e664054d83f3f9451c9b5c9a63e4eca89f5ea6a04c023f
created: 2026-10-22
---

# PRD: OpenCode sessions alongside Claude Code

## 0. Source prompt

> I want to add an ability to run opencode as well as claude code

## 1. Problem

ccctl runs only Claude Code. The owner already uses OpenCode on the same host, and sometimes wants OpenCode rather than Claude Code for a task. Today that means leaving the ccctl page and opening a separate terminal or SSH session: the OpenCode task does not join the six-pane grid, does not get a ccctl-created worktree, gets no amber Needs input pane, and is not relaunched when the controller restarts. The cost is one more terminal to watch and a second remote workflow for the same job. Evidence: the user request in §0 and OpenCode 2.1.3 installed at `/opt/homebrew/bin/opencode` on the host (checked 2026-10-22).

## 2. Target user

- **Primary:** the existing ccctl owner, one developer supervising agent sessions on their own macOS or Linux host from another device on the same tailnet.
- **Secondary:** none in this release; the permission model remains single-owner.
- **Explicitly not for:** other tailnet members, teammates, read-only viewers, and phone-sized screens, which the existing controller already excludes.
- **Job-to-be-done:** When I want a task run by OpenCode instead of Claude Code, I want to start it in the same six-pane grid with the same worktree, upload, stop, and needs-input behavior, so I can mix agents without a second remote workflow.

## 3. Outcome and success metric

- **Outcome this moves:** the number of tasks the owner can supervise under ccctl is no longer limited to tasks they want Claude Code to run.
- **Primary KPI:** count of `session_started` log events with `agent="opencode"` on the host, read from the ccctl log, in the 14 days after this release is installed. A mixed-agent day is a UTC day with at least one `session_started` event for each agent.
- **Baseline:** 0 OpenCode launches; the current `session_started` event has no `agent` property (checked 2026-10-22).
- **Target:** at least 3 OpenCode `session_started` events and at least 1 mixed-agent day by day 14 after install.
- **Guardrail metrics:** every existing Claude acceptance scenario stays green in CI; zero `session_started` events whose `agent` differs from the launch request; zero requests served to a Tailscale login outside the allowlist; zero worktrees removed by ccctl.

## 4. Goals and non-goals

**Goals**
1. Let the owner choose Claude Code or OpenCode for each new session, and mix both agents across the six existing slots (DL-001, DL-004).
2. Give OpenCode sessions the existing worktree, upload, scrollback, stop, close, Tailscale, and restart behavior without a second controller or listener.
3. Give OpenCode panes the same amber Needs input signal, tab-title count, and chime as Claude panes, through an OpenCode plugin that posts to the existing hook socket (DL-002).
4. Relaunch OpenCode sessions after a controller or host restart with the same session ID in the same folder (DL-003).
5. Keep every existing Claude Code launch, resume, and settings path unchanged.

**Non-goals**
1. Manual Resume by ID for OpenCode: the existing Resume by ID tab stays Claude Code only in this release; the owner gets restart resume through the stored session ID (DL-003).
2. Agents other than Claude Code and OpenCode: the agent field accepts exactly `claude` and `opencode`.
3. Renaming ccctl, `~/.ccctl`, the service labels, or the GitHub repository (DL-007).
4. Model, provider, or OpenCode agent selection in the launch form: OpenCode's own configuration chooses the model and agent.
5. Per-agent concurrency limits: all panes share the existing six slots (DL-004).
6. Driving OpenCode through its HTTP server, web UI, ACP server, or database: each pane is the OpenCode TUI in a PTY.
7. Reading, exporting, or migrating OpenCode session data.
8. Controlling more than one host from one page, and every other exclusion already recorded in the base PRD.

## 5. User stories and acceptance criteria

Terms: an **agent** is `claude` or `opencode`. A **session** is one agent process in a PTY owned by the controller. States, slot allocation, worktree naming, and the owner-only permission matrix are unchanged from the base controller (`internal/session/manager.go`, `internal/session/rules.go`, `docs/prd/claude-code-controller.md` §5).

- **[P0]** As the owner, I want to launch either agent into a free slot for a named task, so that each task runs with the agent I choose.
  - [ ] Given the New session form with agent OpenCode, task `fix-login`, repo `~/Documents/folio`, worktree on, and first prompt `hello world`, when I press Start, then within 5 seconds slot 1 shows the OpenCode TUI whose working directory is `~/Documents/folio-wt-fix-login` on branch `ctl/fix-login`, started with `--session` set to a ccctl-generated `ses_...` ID (OpenCode requires the `ses` prefix), `--auto`, and `--prompt "hello world"`, and `state.json` records `agent: "opencode"` with that session ID.
  - [ ] Given the same form with agent Claude Code, when I press Start, then the argv is the existing `claude --dangerously-skip-permissions --session-id ID --settings FILE [prompt]`, and `state.json` records `agent: "claude"`.
  - [ ] Given a launch request whose `agent` field is absent, when the controller accepts it, then the session starts as `claude`, so existing API clients keep working.
  - [ ] Given worktree isolation is on and the folder is a git repository, when I launch either agent, then the worktree is created with the existing sibling-folder and `ctl/TASK` branch rules.
  - [ ] Given all six slots are active with any mix of agents, when I press Start, then the form is disabled with `All 6 slots are in use. Stop or close a session first.` and a direct API request returns HTTP 409 with the same text.
- **[P0]** As the owner, I want an OpenCode pane to flag when it needs me, so that I do not have to read every pane.
  - [ ] Given an OpenCode session launched by ccctl, when OpenCode emits `permission.asked` or `session.idle`, then within 2 seconds that pane's header turns amber with `Needs input`, the tab title is prefixed with the count of such panes, and one chime plays unless muted.
  - [ ] Given a pane in `needs-input`, when any keystroke is sent to it, then the state returns to `running` through the existing input path.
  - [ ] Given the OpenCode plugin is missing or `CCCTL_SLOT` is unset, when OpenCode emits those events, then the plugin exits without posting, other slots are unaffected, and OpenCode keeps running.
  - [ ] Given a hook event arrives with a session ID that does not match the slot's current session, when the controller handles it, then the event is ignored, as the existing Claude hook path already does.
- **[P0]** As the owner, I want each pane to say which agent it runs, so that I can tell the sessions apart at a glance.
  - [ ] Given any live or exited session, when the pane renders, then its header shows `Claude Code` or `OpenCode` next to the task name, and the accessible pane name includes the agent.
  - [ ] Given a Claude session saved before this release, when it is loaded, then its pane shows `Claude Code` because a missing `agent` reads as `claude`.
  - [ ] Given an OpenCode pane in `starting`, then the overlay reads `Starting OpenCode in PATH`; a Claude pane keeps `Starting claude in PATH`.
- **[P0]** As the owner, I want OpenCode sessions to come back after a controller or host restart, so that a reboot does not lose the task.
  - [ ] Given 3 sessions in `running` or `needs-input`, including at least 1 OpenCode session, when the controller stops and starts again, then each is relaunched in its original slot and folder with its recorded agent, before any browser connects; the OpenCode relaunch uses `opencode --session SESSION_ID --auto`.
  - [ ] Given a resumed OpenCode process exits with a non-zero code within 5 seconds, when this happens, then the pane shows `Could not resume TASK (exit code N).` with Start fresh and Close, and writes a `resume_failed` log line carrying `agent`.
  - [ ] Given a pane in `resume-failed` for task TASK, when I press Start fresh, then a new process of the same agent starts in the same slot, working directory, worktree, and branch, with a new session ID, and the state file records the new ID.
  - [ ] Given a session the owner stopped or that exited before the restart, when the controller starts, then that session is not relaunched.
- **[P1]** As the owner, I want install and uninstall to handle the OpenCode binary and plugin, so that setup stays one command.
  - [ ] Given OpenCode is on PATH, when I run `ccctl install`, then it records the absolute `opencode_path` and writes the ccctl plugin to the OpenCode config directory, and exits 0.
  - [ ] Given OpenCode is absent, when I run `ccctl install`, then it skips the path and plugin, exits 0, and Claude Code launches keep working; an OpenCode launch later shows `OpenCode was not found at PATH. Set opencode_path in ~/.ccctl/config.toml.`
  - [ ] Given `~/.config/opencode/plugins/ccctl.ts` already exists without ccctl's marker, when I run `ccctl install`, then it leaves the file untouched, logs `opencode_plugin_conflict`, and prints a warning that OpenCode panes will have no Needs input signal.
  - [ ] Given the plugin carries ccctl's marker, when I run `ccctl uninstall`, then the plugin file is removed and every other OpenCode file is untouched.
- **[P1]** As the owner, I want the launch form to make the Resume by ID limit clear, so that I do not try an unsupported OpenCode resume.
  - [ ] Given the launch form with agent OpenCode selected, when I switch to Resume by ID, then the tab shows `Resume by ID is Claude Code only in this release.` and the submit button is disabled.
  - [ ] Given agent Claude Code selected, when I switch to Resume by ID, then the existing transcript lookup and launch behavior is unchanged.
  - [ ] Given the owner chose OpenCode in an earlier launch from the same browser, when the form opens again, then OpenCode is preselected; a first-time browser opens with Claude Code selected.
- **[P2]** As the owner, I want the documentation to describe both agents, so that setup and limits are clear.
  - [ ] Given the README and `docs/design/protocol.md`, when I read the session and install sections, then both agents, the `agent` API field, the `opencode_path` config key, the OpenCode version floor, and the Resume by ID limit are stated.

Priorities: **P0** release fails without it · **P1** ship-blocking unless waived · **P2** follow-up.

## 6. Experience notes

- **Surface:** the existing single-page web app, REST API, WebSocket, `ccctl` CLI, and service install. No new screen, listener, or background daemon.
- **Launch form:** a segmented `Agent` control with `Claude Code` and `OpenCode` sits above the existing `Start new` / `Resume by ID` control. The last choice is stored per browser under one localStorage key and defaults to `Claude Code` on a first visit.
- **Resume by ID:** with OpenCode selected, the tab shows the exact note `Resume by ID is Claude Code only in this release.` and submit is disabled. With Claude Code selected, the existing lookup card is unchanged.
- **Pane header:** the agent name appears as a small uppercase label next to the task name. The existing state badge, branch, and action buttons keep their positions.
- **Loading and empty states:** an OpenCode pane in `starting` shows `Starting OpenCode in PATH`; Claude keeps `Starting claude in PATH`. The first-prompt placeholder reads `What should Claude work on?` or `What should OpenCode work on?` with the selected agent.
- **Error states:** the exact texts in §5 and the existing 400/409 mapping. A missing OpenCode binary is a form error before any slot changes.
- **Platforms:** latest stable desktop Chrome, Safari, and Firefox at viewports 1280x720 and larger. Phone layouts stay out of scope.
- **Accessibility bar:** WCAG 2.2 AA for the new selector and pane label: keyboard reachable, visible focus, labelled group, and the agent included in the pane's accessible name. Existing terminal and live-region behavior is unchanged.

## 7. Scope

**In scope:** an `agent` field on the launch request and slot info; OpenCode launch and restart resume; a per-agent binary check; an OpenCode plugin that posts needs-input events to the existing Unix socket; state file version 2 with `agent`; install and uninstall handling of the plugin; README, protocol, and UI copy for both agents.

**Out of scope:** every item in §4, plus any change to the base controller's Tailscale, auth, worktree, upload, or scrollback behavior.

**Dependencies:** Claude Code installed and logged in on the host, as today; OpenCode 2.1.3 or newer installed and authenticated on the host for OpenCode sessions; the owner's existing OpenCode configuration supplies the model and provider; git 2.20 or newer for worktrees.

**Constraints:** Go backend and Vue 3 + xterm.js frontend embedded in one binary; no new network listener; the hook plugin talks only to the existing local Unix socket; OpenCode sessions start with `--auto`; six slots total; state format version 2; no product rename.

## 8. Risks and open questions

| ID | Risk or question | Type | Owner | Resolve by |
|---|---|---|---|---|
| R-001 | OpenCode changes plugin event names or the `--session` create behavior in a later major version, so needs-input or restart resume breaks. | Risk | maintainer | Recheck on each OpenCode release; the version floor and fake-binary tests catch launch changes, not event renames. |
| R-002 | The global OpenCode plugin runs for OpenCode sessions started outside ccctl; a bug there could add latency to those sessions. | Risk | maintainer | The plugin no-ops when `CCCTL_SLOT` is unset and posts with a 2-second timeout. |
| R-003 | A host with a custom `OPENCODE_CONFIG_DIR` that changes after install gets no plugin. | Risk | owner | Rerun `ccctl install`; the recorded `opencode_config_dir` is used by every session. |
| R-004 | An older ccctl refuses a version 2 state file, so downgrading needs a manual state reset. | Risk | owner | Documented rollback in §10; the release checklist exercises it. |

## 9. Instrumentation

| Event | Trigger | Properties | Purpose |
|---|---|---|---|
| `session_started` | A session reaches `running` | `slot`, `task`, `cwd`, `worktree`, `branch`, `session_id`, `resumed`, `agent` | Primary KPI and guardrail that the agent matches the request. |
| `session_exited` | The process exits | `slot`, `task`, `exit_code`, `duration_s`, `stopped_by_user`, `agent` | Guardrail for launch and runtime failures by agent. |
| `needs_input` | A hook or plugin event flips a pane | `slot`, `task`, `hook`, `agent`, `source` (`claude-hook` or `opencode-plugin`) | Proves the plugin path works and measures the signal delay. |
| `resume_failed` | A resumed process exits non-zero within the grace window | `slot`, `task`, `session_id`, `exit_code`, `agent` | Restart-resume guardrail by agent. |
| `opencode_plugin_written` | `ccctl install` writes or updates the plugin | `path`, `opencode_version` when known | Setup status. |
| `opencode_plugin_conflict` | Install finds a non-ccctl file at the plugin path | `path` | Explains a missing needs-input signal. |
| `opencode_plugin_removed` | `ccctl uninstall` removes the plugin | `path` | Confirms cleanup. |

The controller writes these to its existing log on the host; there is no external analytics service.

## 10. Rollout and rollback

- **Strategy:** full release in the next `v0.1.x` tag through the existing PR and CI flow. No feature flag: the binary is single-owner, and an install replaces the service binary and restarts it.
- **Rollback trigger:** revert and reinstall the previous release if any existing Claude acceptance scenario fails on `main`, or if two consecutive launches of either agent on the host fail with a non-zero exit before the TUI appears.
- **Migration or backfill:** `state.json` moves from version 1 to version 2 on the first save after launch. Version 2 reads a version 1 file and treats a missing `agent` as `claude`; no backfill command is needed. Rolling back to a version 1 binary requires stopping all sessions, moving `~/.ccctl/state.json` aside, and reinstalling the previous release, which loses automatic restart resume for the sessions that were live. The release checklist exercises this procedure on macOS and Linux.

## 11. Compliance gates

- [x] None apply beyond the existing controller gates: this release adds no payment, no personal-data field, no new network service, and no model call by ccctl. OpenCode's model use stays under the owner's existing OpenCode account and terms; ccctl does not read or store its credentials.

## 12. Data and integrations

- **Stored:** `state.json` version 2 gains `agent` on each slot (`claude` or `opencode`) and keeps the existing fields. An OpenCode slot's `session_id` starts with `ses_`; a legacy UUID in an OpenCode slot is replaced at resume. `~/.ccctl/config.toml` gains optional `opencode_path` and `opencode_config_dir`. The generated plugin file is a code artifact with a ccctl marker, not user data.
- **Retention:** state entries live until a pane is closed; the plugin lives until `ccctl uninstall`; ccctl never reads or modifies `opencode.db`, transcripts, or credentials.
- **Personal data:** none added. Task names and folder paths are already stored in the state file with mode 0600.
- **Migration:** version 1 to version 2 on load and save; missing `agent` means `claude`. No backfill and no OpenCode data import.
- **Integrations:**
  - **OpenCode CLI:** launched as a child process in a PTY with `--session ses_... --auto` plus `--prompt TEXT` when a first prompt is given. OpenCode requires every session ID to start with `ses`; ccctl generates `ses_` plus a hyphen-free UUID and repairs a legacy UUID found in a version 2 state file by starting a fresh `ses_` session. Authentication is whatever the host user already configured with `opencode auth`. Tests replace it with a fake OpenCode binary that records argv and prints a terminal banner, following the existing fakeclaude pattern.
  - **OpenCode config directory:** `ccctl install` resolves `$OPENCODE_CONFIG_DIR` when set, else `~/.config/opencode`, writes `plugins/ccctl.ts`, and records a non-default directory as `opencode_config_dir`. The session environment sets `OPENCODE_CONFIG_DIR` from that value so the service loads the same plugin.
  - **Hook socket:** the plugin reads `CCCTL_SOCK` and `CCCTL_SLOT` from its environment and posts `{slot, hook, session_id}` to the existing Unix socket with a 2-second timeout, using the same path as `ccctl hook`. A missing socket or a non-2xx reply is ignored. The plugin file is covered by a golden-content test; the socket handler is covered by the existing hook tests; a manual smoke test against OpenCode 2.1.3 is in the release checklist.
  - **No other integration:** ccctl does not call OpenCode's HTTP API, database, or web UI.

## 13. Non-functional requirements

- **Launch latency:** either agent reaches `running` within 5 seconds at p95 on the host, measured from the POST to the WebSocket slot event.
- **Needs-input latency:** an OpenCode `permission.asked` or `session.idle` event flips the pane within 2 seconds at p95, measured from the plugin post to the WebSocket slot event.
- **Plugin timeout:** the plugin gives up after 2 seconds and never blocks the OpenCode TUI; a failed post is discarded.
- **Listener surface:** 0 new TCP or UDP listeners; the only new traffic is a local Unix-socket POST to the existing `ccctl.sock`.
- **Concurrency:** up to 6 sessions total with any mix of agents; ccctl starts no extra daemon and no OpenCode server of its own.
- **Backward compatibility:** existing REST routes and WebSocket frames keep their shapes; `agent` is additive, an absent launch `agent` means `claude`, and a version 1 state file loads.
- **Security:** the Tailscale WhoIs allowlist and Host/Origin checks are unchanged; OpenCode sessions run with `--auto`; the plugin socket is the existing 0600 file; ccctl reads no OpenCode credentials.
- **Availability:** the controller starts and serves when OpenCode is absent; only OpenCode launches fail, with the named message in §5.
- **Memory and CPU:** a separate budget for OpenCode is out of scope; it shares the host with the same six-slot ceiling as Claude Code.

## 14. Assumptions

| ID | Dimension | Assumption | Why this default | Overturn if |
|---|---|---|---|---|
| A-001 | purpose | Success is measured from host logs, not a dashboard: at least 3 OpenCode `session_started` events and 1 mixed-agent day in the first 14 days. | The controller has no analytics service and its existing log is the only measurement point. | The owner wants a different usage target or a formal dashboard. |
| A-002 | constraints | OpenCode 2.1.3 or newer is required; ccctl checks only that the binary is executable and does not parse its version. | 2.1.3 is installed on the host and supports `--session` create-or-continue, `--prompt`, `--auto`, and the plugin events used here. | A supported version check is wanted, or an older OpenCode must work. |
| A-003 | integrations | OpenCode uses its normal background service; ccctl does not pass `--standalone`. | Running `opencode` in a terminal behaves this way, and multiple TUI clients are the normal OpenCode model. | Concurrent ccctl sessions interfere through the shared service. |
| A-004 | integrations | Install resolves a custom `OPENCODE_CONFIG_DIR` once and records it; changing that directory later requires rerunning install. | The service does not load the owner's shell environment, matching how `claude_path` is recorded. | A runtime lookup from the owner's shell is required. |
| A-005 | errors | If the plugin cannot be installed, OpenCode sessions still launch and only the Needs input signal is unavailable; install logs and prints a warning. | A missing signal is less damaging than blocking every OpenCode launch, and the warning names the fix. | The owner wants install to fail until the plugin is present. |
| A-006 | data | OpenCode's database and transcripts are never read by ccctl; restart resume relies only on the ID ccctl generated. | Keeps ccctl independent of OpenCode's private storage format and avoids a second parser. | Manual OpenCode resume or transcript display is added later. |

## 15. Decision log

| ID | Dimension | Question | Options offered | Answer | Applied in |
|---|---|---|---|---|---|
| DL-001 | purpose | You wrote "add an ability to run opencode as well as claude code". Which whole reading do you mean? | A. Per-session agent choice (recommended) B. One global agent switch C. Both agents per task D. Separate OpenCode page/grid | A | §1, §2, §4, §5 |
| DL-002 | integrations | How should an OpenCode pane signal that it needs input? | A. Plugin + hook socket (recommended) B. No needs-input in v1 C. Scrape the TUI output D. Use OpenCode's server API | A | §4, §5, §9, §12 |
| DL-003 | inputs | Which resume capability should OpenCode get in this release? | A. Restart resume only (recommended) B. Also manual Resume by ID C. No OpenCode resume | A | §4, §5, §6, §12 |
| DL-004 | constraints | Do OpenCode sessions share the existing six slots with Claude Code? | A. Shared six total (recommended) B. Six per agent, up to 12 C. Shared six with per-agent sub-caps | A | §4, §5, §13 |
| DL-005 | constraints | Should OpenCode sessions start with auto-approved permissions? | A. Always auto-approve (recommended) B. Never auto-approve C. Per-launch toggle | A | §5, §7, §13 |
| DL-006 | data | How should ccctl find and configure the OpenCode binary? | A. Optional auto-detect (recommended) B. Require both at install C. Add path fields to Settings | A | §5, §12 |
| DL-007 | non_goals | Does this release change the ccctl product name and paths? | A. Keep the name (recommended) B. Rename now C. Keep name, neutral copy | A | §4, §7 |
| DL-008 | interface | Where does the owner choose the agent, and what happens on the Resume by ID tab? | A. Selector with resume note (recommended) B. Selector in Start new only C. Choose after launch | A | §5, §6 |
| DL-009 | data | How should the state file record each pane's agent? | A. Bump to version 2 (recommended) B. Keep version 1 C. Separate agents file | A | §5, §10, §12 |
| DL-010 | integrations | What should ccctl do with the OpenCode plugin file on install and uninstall? | A. Install, remove on uninstall (recommended) B. Install only C. Refuse on conflict | A | §5, §9, §12 |
| DL-011 | acceptance | Which stories must be P0 for this release? | A. First four stories: launch/mix, needs-input, agent label, restart resume (recommended) B. First three only; restart resume is P1 C. Launch/mix only is P0 | A, with install/uninstall and the resume note at P1 and documentation at P2 | §5 |

## Coverage

| Dimension | Status | Ref |
|---|---|---|
| purpose | assumed | §1, §3, §4, A-001 |
| actors | specified | §2, §5 |
| permissions | specified | §5, §7 |
| triggers | specified | §5, §6 |
| inputs | specified | §5, §12 |
| outputs | specified | §5, §6, §9 |
| states | specified | §5 |
| errors | specified | §5, §12, A-005 |
| interface | specified | §5, §6 |
| data | specified | §10, §12, A-006 |
| integrations | specified | §12, A-003, A-004 |
| non_functional | specified | §13 |
| constraints | specified | §7, §13, A-002 |
| non_goals | specified | §4 |
| acceptance | specified | §5 |
| rollout | specified | §10 |
