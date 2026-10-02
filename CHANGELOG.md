# Changelog

Notable changes to ccctl. Release binaries and their notes are on the [Releases page](../../releases); every merge to `main` publishes a release tagged `v0.1.N`.

## Unreleased

- The project is now named ccctl and lives at `github.com/ryabinski-labs/ccctl` (it was `claude-code-controller`), so its name no longer contains Anthropic's "Claude Code" trademark. GitHub redirects the old URLs, so existing installs keep upgrading.
- New session lists repos only from the folder you set in Settings (`repo_prefix` in `config.toml`). Nothing is scanned until it is set, and the old `roots` setting and the scan of `~` are gone. A scan that takes more than 10 seconds, for example while macOS waits for a privacy prompt, now stops with an explanation and a Retry button instead of showing "Scanning folders…" forever.
- Terminal text is 10 px (was 13 px), the same cell width as Monaco 10 in macOS Terminal, so each pane shows more columns and rows.
- Run up to six sessions (was four). The pane grid now grows with the number of open sessions: one pane fills the page, two sit side by side, three or four make a 2x2, five or six make a 3x2. The "Start a session" card only appears when the grid has a spare cell, and `Ctrl+Shift+1` to `6` focus the panes.
- Pane buttons are now 32 px, the attach button keeps keyboard focus and shows a spinner while a file uploads, and an upload error shows above the terminal (with a dismiss button) instead of covering it.
- The page has a real `<h1>`, and missing file-like paths such as `/robots.txt` return 404 instead of the page.
- README: phone layouts are not supported (the page needs a window at least 1024 px wide).
- Send files and images from the viewing device into a session: paste or drop a file on a pane, or use the paperclip button. The file is saved on the host under `~/.ccctl/uploads` and its path is typed into the session. Uploads older than 14 days are removed, and the folder is capped at 1 GB.
- Open-source release preparation: MIT license, community files, CI on GitHub-hosted runners, and release downloads that need no GitHub token.
