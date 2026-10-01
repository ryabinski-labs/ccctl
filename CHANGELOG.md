# Changelog

Notable changes to ccctl. Release binaries and their notes are on the [Releases page](../../releases); every merge to `main` publishes a release tagged `v0.1.N`.

## Unreleased

- Send files and images from the viewing device into a session: paste or drop a file on a pane, or use the paperclip button. The file is saved on the host under `~/.ccctl/uploads` and its path is typed into the session. Uploads older than 14 days are removed, and the folder is capped at 1 GB.
- Open-source release preparation: MIT license, community files, CI on GitHub-hosted runners, and release downloads that need no GitHub token.
