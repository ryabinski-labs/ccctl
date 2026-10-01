---
spec: spec-clarifier/v1
feature: repo-prefix
title: Repository folder prefix in Settings
status: complete
prompt: inline
prompt_sha256: 7ef3e6d7b5b41d8983b6681ad66b8ed50d39310999c5e8bb01279059745f7d01
created: 2026-10-01
---

# PRD: Repository folder prefix in Settings

## 0. Source prompt

> scanned repositories don't seem to work, let's allow specifying a prefix in settings (and don't scan repos if it's not set) then only show repos from that prefix.

## 1. Problem

On 2026-10-01 the owner opened "New session in slot 1" and the Folder list showed `Scanning folders…` and never produced a list (screenshot `Screenshot 2026-10-01 at 10.49.16 AM.png`). The list is filled by `GET /api/repos`, which walks `~/projects` and `~/Documents` three levels deep plus every direct child of `~` (`internal/repos/repos.go`, `internal/config/config.go`). The README documents that on macOS the first scan of `~/Documents` waits on a privacy prompt shown only on the host's screen (`README.md` line 102), so the request does not return and the dialog has no timeout (`web/src/components/LaunchDialog.vue`, `reposLoading`). Cost today: the owner cannot pick a repo from the list and falls back to typing a full path.

Evidence: the owner's screenshot and message; repo pointers above. Root cause (privacy prompt) is the likely reading, not confirmed on the owner's host.

## 2. Target user

- **Primary:** the owner, a Tailscale login in `allowed_logins`, starting sessions from a browser on any tailnet device.
- **Secondary:** none; the controller has one role (existing PRD DL-006).
- **Explicitly not for:** callers outside `allowed_logins`; they receive HTTP 403 as for every other route.
- **Job-to-be-done:** When I start a session, I want the list to show only repos under the one folder I keep my work in, so I can pick one without waiting on a scan of folders I never use.

## 3. Outcome and success metric

- **Outcome this moves:** time from opening the New session dialog to a usable Folder list.
- **Primary KPI:** milliseconds from `GET /api/repos` request to response, logged as `repos_scan` `duration_ms` (section 9).
- **Baseline:** no response at all on the owner's host on 2026-10-01 (screenshot).
- **Target:** response within 10,000 ms for 100 percent of requests (the server stops the scan at 10 s), and p95 under 1,000 ms for a folder holding up to 50 repos, measured by a Go test on the host.
- **Guardrail metrics:** launching with a custom path still succeeds; a session started from the list still gets the same worktree and branch names as before.

## 4. Goals and non-goals

**Goals**
1. Settings holds one repository folder (the prefix); only repos under it are scanned and listed.
2. With no prefix set, no scan runs and the dialog says how to set one.
3. A scan that does not finish in 10 seconds ends with an explanation and a retry, never an endless wait.

**Non-goals**
1. Several folders: one folder only (DL-001). Reason: smallest build that matches the prompt.
2. Filtering by repo-name prefix: not what the owner asked for (DL-001).
3. Restricting custom paths to the prefix: the custom path field stays unrestricted so any folder remains reachable (A-002).
4. A folder picker or browse dialog: the field takes typed text. Reason: not requested.
5. Streaming partial scan results: the dialog waits for the full list or the timeout (DL-005).

## 5. User stories and acceptance criteria

Terms: the **prefix** is the single folder path stored in `config.toml` under `repo_prefix` and edited in Settings. Listed repos are git repos whose main working tree (`.git` is a folder) sits within 3 folder levels below the prefix, counting the prefix folder as level 0, matching the existing walk in `internal/repos/repos.go`. Permission: the owner only, the same allowlist check as `PUT /api/settings/env/{name}` (A-005).

- **[P0]** As the owner, I want to set the repository folder in Settings, so that the launch list shows only repos from it.
  - [ ] Given no prefix is set, when I open Settings, enter `~/Documents/projects`, and press Save, then the field shows `~/Documents/projects`, a confirmation reads `Repository folder saved. It applies the next time you open New session.`, and `config.toml` contains `repo_prefix = "~/Documents/projects"` with every other key unchanged.
  - [ ] Given the prefix `~/Documents/projects` holds repos `a` and `b/c` and a repo `~/other/d` exists outside it, when I open New session, then the list shows `a` and `b/c` and does not show `d`.
  - [ ] Given a prefix that is set, when I clear the field and press Save, then `repo_prefix` is removed from `config.toml` and the next New session dialog behaves as if no prefix was ever set (A-004).
  - [ ] Error: given I enter `/no/such/folder` or a path that is not an absolute folder after `~` expansion, when I press Save, then Settings shows `Folder /no/such/folder does not exist or is not a folder.` under the field and `config.toml` is unchanged (DL-003).
  - [ ] Error: given a cross-origin request to the repo folder endpoint, when it is sent, then it is refused with HTTP 403 `Cross-origin request refused.` and `config.toml` is unchanged (A-005).
- **[P0]** As the owner, I want no scan to run until a prefix is set, so that no folder I did not choose is read.
  - [ ] Given no prefix is set, when I open New session, then no `GET /api/repos` scan walks any folder, the list area shows `No repository folder is set.` with an `Open Settings` button, and the custom path link is still present (DL-004).
  - [ ] Given no prefix is set and `config.toml` still holds an old `roots` list, when the controller starts and I open New session, then no folder in `roots` and no direct child of `~` is read (DL-002).
  - [ ] Given no prefix is set, when I press `Open Settings`, then the Settings dialog opens with focus in the repository folder field.
- **[P0]** As the owner, I want a slow scan to stop with an explanation, so that the dialog is never stuck on `Scanning folders…`.
  - [ ] Error: given the scan under the prefix has not finished after 10 seconds, when the timer ends, then the server returns HTTP 504 and the list area shows `Scanning took too long. On a Mac, allow ccctl to access this folder in the prompt on the host, then retry.` with a `Retry` button, and the custom path link remains (DL-005).
  - [ ] Given the timeout message is showing, when I press `Retry`, then the dialog shows `Scanning folders…` and sends one new scan request.
- **[P0]** As the owner, I want a clear message when the prefix holds no repos, so that I know the setting is the reason.
  - [ ] Given the prefix folder exists and holds no git repos within 3 levels, when I open New session, then the list area shows `No repositories found under ~/Documents/projects. Use a custom path or change the folder in Settings.`
  - [ ] Given the prefix folder was deleted after it was saved, when I open New session, then the list area shows `The repository folder ~/Documents/projects was not found. Change it in Settings.` and no scan error is logged as a failure (A-003).
- **[P1]** As the owner, I want the search box to keep working on the filtered list, so that I can narrow it by name.
  - [ ] Given the prefix lists 12 repos, when I type `fol` in `Search repositories`, then only repos whose name or display path contains `fol` remain.
- **[P2]** As the owner, I want the README to describe the new setting, so that an upgrade from `roots` is not a surprise.
  - [ ] Given the README configuration table, when I read the `repo_prefix` row, then it states the default (unset, no scan), the 3-level depth, and that `roots` is ignored.

Priorities: **P0** release fails without it · **P1** ship-blocking unless waived · **P2** follow-up.

## 6. Experience notes

- **Surface:** the existing web UI. Two screens change: the Settings dialog (`web/src/components/SettingsDialog.vue`) gains a "Repository folder" field with Save and a Clear control above the environment variables; the New session dialog's Folder list (`LaunchDialog.vue`) gains the states below.
- **Folder list states:** unset (message plus `Open Settings`), loading (`Scanning folders…`), timed out (message plus `Retry`), prefix missing, empty prefix, populated. The link `Use a custom path` is present in every state.
- **Settings field:** a text input labelled `Repository folder`, placeholder `~/projects`, a hint `Only git repos under this folder are listed when you start a session. Leave empty to turn scanning off.` Errors appear under the field with `role="alert"`; the success message uses `role="status"`.
- **Accessibility bar:** WCAG 2.2 AA for the controller chrome, the same bar as the existing PRD section 6 (A-007). The `Open Settings` and `Retry` buttons are keyboard reachable with visible focus, and the timed-out and unset messages are announced with `role="status"`.
- **Viewport:** desktop windows 1024 px wide or more; phone layouts stay a non-goal.

## 7. Scope

**In scope:**
- A `repo_prefix` key in `config.toml`, set and cleared from Settings.
- `GET` and `PUT` on a repo-folder settings endpoint and `DELETE` to clear it.
- Scanning only under the prefix, with a 10-second server-side stop.
- Removing the `roots` default and the direct-children-of-`~` check.
- Launch dialog states, README and CHANGELOG updates, tests.

**Out of scope:** more than one folder; name-prefix filtering; restricting custom paths; a folder picker; partial results; migrating old `roots` values.

**Dependencies:** none beyond the existing config writer pattern in `internal/config/env.go`.

**Constraints:** Go and Vue 3 stack as is; no new dependency; `config.toml` stays mode 0600 and other keys are preserved on write; the `/api/` allowlist check and Origin check apply to the new routes.

## 8. Risks and open questions

| ID | Risk or question | Type | Owner | Resolve by |
|---|---|---|---|---|
| R-001 | The owner's prefix may sit under `~/Documents`, so the macOS privacy prompt can still block the first scan; the 10-second timeout and message are the mitigation. | Risk | owner | 2026-10-08 |
| R-002 | Users upgrading with a `roots` entry get an empty list until they set a prefix; the README and the unset message explain it. | Risk | owner | 2026-10-08 |

## 9. Instrumentation

All events are JSON log lines on the host only, as in the existing PRD section 9.

| Event | Trigger | Properties | Purpose |
|---|---|---|---|
| settings_repo_prefix_changed | the prefix is set or cleared | action (set or cleared), login | audit of the setting |
| repos_scan | a scan ends | duration_ms, count, timed_out | the KPI in section 3 |

## 10. Rollout and rollback

- **Strategy:** full release to the owner's hosts. The merge to `main` publishes the next `v0.1.N` release and the owner runs `ccctl install` (A-006).
- **Rollback trigger:** any P0 acceptance criterion in section 5 failing on a host after the upgrade. Rollback is `ccctl install --version v0.1.17`, which restores the `roots` scan; `repo_prefix` stays in `config.toml` and the older binary ignores it.
- **Migration or backfill:** none. An existing `roots` key is ignored and left in the file untouched (DL-002).

## 11. Compliance gates

- [x] None apply: a single-user developer tool on the owner's own hosts that collects no data about other people; the new setting is a local folder path in a file only the owner can read.

## 12. Data and integrations

- **Stored:** one string key `repo_prefix` in `~/.ccctl/config.toml` (mode 0600), holding the path as typed with a leading `~` kept; read at each request so a change needs no restart (A-004). The path is not personal data about anyone else.
- **Migration:** none (section 10).
- **Integrations:** none external. Reuses the atomic config writer pattern from `config.SetEnv` (`internal/config/env.go`) and the route allowlist in `internal/server/server.go`. Tests use a temporary home folder with created `.git` folders, as in `internal/repos` tests; no network.
- **API (A-001):** `GET /api/settings/repo-prefix` returns `{"prefix": "<string or empty>"}`; `PUT` with `{"prefix": "<path>"}` validates and stores it; `DELETE` removes it; `GET /api/repos` walks nothing when the prefix is unset, returns HTTP 504 with `{"error": "..."}` when the scan passes 10 seconds, `{"repos": [], "prefix": "<string>", "missing": true}` when the prefix folder does not exist (A-003), and `{"repos": [...], "prefix": "<string>"}` otherwise; an unset prefix returns `{"repos": [], "prefix": ""}`.

## 13. Non-functional requirements

- Scan stop: the server ends a scan 10 seconds after it starts, measured by a Go test with a blocked filesystem double.
- Scan time p95 under 1,000 ms for a prefix holding 50 repos within 3 levels, measured by a Go benchmark on the host.
- Unset prefix: 0 directory reads on `GET /api/repos`, measured by a Go test with a counting filesystem double.
- Config write: `config.toml` keeps mode 0600 and all other keys after 100 percent of saves, measured by a Go test.
- Security: the PUT and DELETE routes require the allowlist check and the page's own Origin on 100 percent of requests.

## 14. Assumptions

| ID | Dimension | Assumption | Why this default | Overturn if |
|---|---|---|---|---|
| A-001 | integrations | The setting is stored as `repo_prefix` in `config.toml` and exposed at `/api/settings/repo-prefix` (GET, PUT, DELETE). | Mirrors the existing `/api/settings/env` routes and the `config.SetEnv` writer. | The owner wants a different key or route name. |
| A-002 | inputs | The custom path field is not limited to the prefix. | The owner asked to limit the list, not launching; a custom path is the escape hatch (DL-004). | The owner wants every launch confined to the prefix. |
| A-003 | errors | A saved prefix that no longer exists shows `The repository folder <path> was not found. Change it in Settings.` instead of an empty list. | A missing folder and an empty folder need different fixes. | The owner prefers one generic empty message. |
| A-004 | states | A saved or cleared prefix applies from the next `GET /api/repos`; no restart is needed. | Config is re-read per request, as for `[env]`. | Caching the scan is wanted for speed. |
| A-005 | permissions | Only allowlisted logins may read or change the setting, and PUT and DELETE also require the page's own Origin. | Same as `PUT /api/settings/env/{name}` in `internal/server/server.go`. | The setting becomes visible to other roles. |
| A-006 | rollout | No feature flag; the owner upgrades each host with `ccctl install`. | Matches existing PRD DL-014 and A-014. | A host must keep the old scan during a trial. |
| A-007 | interface | WCAG 2.2 AA applies to the new field, buttons and messages. | The existing PRD A-012 sets the bar for the chrome. | A different accessibility bar is required. |
| A-008 | inputs | Walk depth stays 3 folder levels below the prefix, repos are not descended into, and linked worktrees (`.git` is a file) are not listed. | Unchanged behavior of `internal/repos/repos.go`; existing PRD A-008. | The owner wants deeper or shallower scans. |
| A-009 | inputs | If the prefix folder itself is a git repo it is listed as one entry. | A prefix set directly to one repo should still be pickable. | The owner wants only children listed. |
| A-010 | purpose | The only success number is the scan time and timeout in section 3; no product metric is added. | A bug fix and a setting, not a growth feature. | A usage metric is wanted. |
| A-011 | inputs | A PUT to the repo folder endpoint with no folder text returns HTTP 400 `Enter a folder path.`, and the Settings field sends DELETE when it is empty. | An empty save means clear, and the API still needs a defined answer. | The owner wants an empty save to be refused in the field. |
| A-012 | interface | Open Settings closes the New session dialog, then opens Settings with focus in the folder field; the Settings dialog is titled `Settings` and keeps the environment variables under the heading `Session environment`. | Two stacked dialogs would trap focus. | The owner wants the launch form kept open. |
| A-013 | errors | A scan failure that is not a timeout shows `The controller could not list repositories. Use a custom path or retry.` with a Retry button. | The timeout message names a cause that may not apply. | The owner wants the timeout message for every failure. |
| A-014 | outputs | A Clear button appears beside Save folder while a folder is saved; clearing shows `Repository folder cleared. New session will not scan until you set one.` | Section 6 asks for a Clear control; the wording matches the saved message. | The owner wants different wording. |

## 15. Decision log

| ID | Dimension | Question | Options offered | Answer | Applied in |
|---|---|---|---|---|---|
| DL-001 | purpose | Which reading of "a prefix in settings" is meant? | A. One folder path in Settings (recommended) B. Several folder paths C. Keep roots scan and filter by path prefix D. Filter by repo-name prefix | A | §1, §2, §4, §5 |
| DL-002 | constraints | What happens to the existing roots key and the direct-children-of-home check? | A. Remove both (recommended) B. Keep roots as a fallback C. Migrate the first root once | A | §5, §7, §10 |
| DL-003 | inputs | What does Settings do when the saved folder is not an existing absolute folder? | A. Reject with a message (recommended) B. Save anyway C. Save then warn | A | §5 |
| DL-004 | interface | What does the Folder list show when the setting is unset? | A. Message plus Settings button (recommended) B. Message only C. Open the custom path field directly | A | §5, §6 |
| DL-005 | errors | What should the dialog do when the scan has not answered after 10 seconds? | A. Stop waiting, explain, offer retry (recommended) B. Keep waiting C. Show partial results | A | §5, §13 |
| DL-006 | acceptance | Which stories must pass for this release to ship? | A. Settings, no scan until set, slow-scan stop, and empty or missing folder messages (recommended: first three) B. First three only C. Settings and no-scan only | A, all four | §5 |

## Coverage

| Dimension | Status | Ref |
|---|---|---|
| purpose | specified | §1, §3, A-010 |
| actors | specified | §2 |
| permissions | assumed | §5, A-005 |
| triggers | specified | §5, §6 |
| inputs | specified | §5, DL-003, A-002, A-008, A-009, A-011 |
| outputs | specified | §5, §12 |
| states | assumed | §5, A-004 |
| errors | specified | §5, DL-005, A-003, A-013 |
| interface | specified | §6, DL-004, A-007, A-012, A-014 |
| data | specified | §12 |
| integrations | assumed | §12, A-001 |
| non_functional | specified | §13 |
| constraints | specified | §7, DL-002 |
| non_goals | specified | §4, §7 |
| acceptance | specified | §5, DL-006 |
| rollout | assumed | §9, §10, A-006 |
