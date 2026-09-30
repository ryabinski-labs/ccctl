# ccctl UI brief (ui-ux-cx-engineer)

**User and job.** One developer supervising up to four Claude Code agents from another desktop.
Top task: glance at four panes, spot the one waiting on them, type into exactly that one.
Second: start a session in a free slot. Failure that matters most: typing into the wrong pane.

**Direction — "control room on paper".** Light, warm chrome (house rule: light theme) around four
dark terminal "monitors". The contrast makes occupancy readable at a glance: a live slot is a dark
well; a free slot is a light dashed card. Claude Code's TUI is designed for a dark background, so
the terminal wells stay dark.

- Type: IBM Plex Sans (chrome), IBM Plex Mono (task, branch, paths, terminal). Self-hosted via
  @fontsource (page is served over the tailnet; no external requests).
- Tokens: `--paper #F5F3EE`, `--panel #FFFFFF`, `--ink #1C1B18`, `--ink-2 #55524B`, `--rule #DAD6CC`,
  `--well #121317`, `--well-fg #E7E5DF`, `--accent #1F4FD1` (primary/focus),
  `--ok #1E7A4C`, `--wait-bg #FDECC8`, `--wait-fg #7A3E00`, `--wait-rule #D98A00`,
  `--err #B42318`, `--muted #7A766C`. All text pairs ≥ 4.5:1.
- Density: header 52px, pane header 36px, 8px grid gutters, 12px page padding. Grid fills viewport.

**Header.** `ccctl` wordmark · host name (mono) with connection dot · summary ("3 running · 1 needs
input") · toggles: Screen reader mode, Chime on/off (icon + text) · primary **New session** (disabled
with title `All 4 slots are in use. Stop or close a session first.` when 4 active).

**Pane.** Header: slot chip `1`, task (mono, semibold), `repo · ctl/branch` (muted, ellipsis),
state badge (dot + text: Starting / Running / Needs input / Exited / Resume failed), icon buttons
Maximize/Restore and Stop (aria-label + title). **Needs input:** the whole header turns amber
(`--wait-bg`, 3px `--wait-rule` top border, badge text `Needs input`) — color plus text, never
color alone. **Focused pane:** 2px `--accent` outline around the pane + header shows `typing here`
hint; this is the guard against typing into the wrong pane.

- Starting overlay (until first output): spinner + `Starting claude in PATH`.
- Exited: terminal stays readable; a bottom bar: `Exited (code N). Worktree kept at PATH on branch
  ctl/TASK.` + **Close pane**.
- Resume failed: centered card over the well: `Could not resume TASK (exit code N).` + **Start
  fresh** (primary) + **Close**.
- Free slot: light dashed card, `Slot N is free`, **Start a session** (opens form bound to slot).

**Launch form (modal dialog, focus-trapped, Esc closes).** Title `New session in slot N`.
Task name (hint: "a-z, 0-9 and -, up to 40"; live preview `Creates ~/Documents/folio-wt-fix-login
on branch ctl/fix-login`). Folder: searchable list of scanned repos (listbox, arrow keys) with
"Use a custom path" text field. Worktree toggle (on by default for git repos; for non-git: off,
disabled, note `Not a git repository: session runs in the folder itself.`). First prompt textarea
with counter `0 / 10,000`. Footer: Cancel · **Start session**. Server errors render verbatim in an
error block above the footer and move focus there. Smart default: preselect the last-used folder
(localStorage, per viewer).

**Stop dialog.** `Stop TASK? The worktree and branch are kept.` Initial focus on Cancel; danger
button **Stop session**.

**Reconnect banner.** Full-width bar under the header: `Lost connection to HOST. Check that
Tailscale is on for this device and for HOST. Retrying in N s.` Countdown updates visually; the
live region announces only the first message.

**Alerts.** Tab title `(N) ccctl · HOST` when N panes need input. One soft two-tone WebAudio chime
per transition into needs-input, unless muted (mute persisted in localStorage).

**Keyboard (WCAG 2.1.2 — terminals capture Tab).** `Ctrl+Shift+1..4` focus pane N;
`Ctrl+Shift+0` leave terminals to the header; `Ctrl+Shift+M` maximize/restore focused pane.
Shown in a "Keys" popover in the header. Every chrome control keyboard-reachable with a visible
2px focus ring. State changes announced through one `aria-live="polite"` region
("Slot 2 fix-login needs input").

**Desktop only**, ≥1280×720 (phones are a non-goal).
