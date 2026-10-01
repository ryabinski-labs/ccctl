# Changelog

Notable changes to ccctl. Release binaries and their notes are on the [Releases page](../../releases); every merge to `main` publishes a release tagged `v0.1.N`.

## Unreleased

- Terminal text is 11 px (was 13 px), so each pane shows more columns and rows. At six panes on a 1280 px window a pane now fits about 65 columns (was about 52).
- Run up to six sessions (was four). The pane grid now grows with the number of open sessions: one pane fills the page, two sit side by side, three or four make a 2x2, five or six make a 3x2. The "Start a session" card only appears when the grid has a spare cell, and `Ctrl+Shift+1` to `6` focus the panes.
- Pane buttons are now 32 px, the attach button keeps keyboard focus and shows a spinner while a file uploads, and an upload error shows above the terminal (with a dismiss button) instead of covering it.
- The page has a real `<h1>`, and missing file-like paths such as `/robots.txt` return 404 instead of the page.
- README: phone layouts are not supported (the page needs a window at least 1024 px wide).
- Send files and images from the viewing device into a session: paste or drop a file on a pane, or use the paperclip button. The file is saved on the host under `~/.ccctl/uploads` and its path is typed into the session. Uploads older than 14 days are removed, and the folder is capped at 1 GB.
- Open-source release preparation: MIT license, community files, CI on GitHub-hosted runners, and release downloads that need no GitHub token.
