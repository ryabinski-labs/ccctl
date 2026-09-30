# claude-code-controller (`ccctl`)

A web controller for up to 4 live Claude Code sessions on one host, reached over Tailscale.
Open `http://TAILSCALE_IP:7681/` from any device on your tailnet to get a 2x2 grid of real
`claude` terminals. You can watch all four, type into any of them, and see which one is
waiting for you.

- **Spec:** `docs/prd/claude-code-controller.md`
- **TDD artifact:** `tdd/claude-code-controller.tdd.yaml`
- **UI brief:** `docs/design/ui-brief.md`
- **Wire protocol:** `docs/design/protocol.md`

## How it works

- **One binary.** `ccctl` is a Go program with the Vue 3 + xterm.js frontend embedded in it.
- **Sessions.** Each session is `claude --dangerously-skip-permissions --session-id UUID --settings FILE [prompt]` running in its own PTY. For git repos, each task gets its own worktree at `<repo>-wt-<task>` on branch `ctl/<task>`, created from HEAD. Worktrees and branches are never removed by ccctl.
- **Needs input.** Claude Code's `Notification` and `Stop` hooks call `ccctl hook --slot N` over `~/.ccctl/ccctl.sock`. The pane header turns amber, the tab title shows a count, and a chime plays (you can mute it).
- **Restarts.** Live sessions are recorded in `~/.ccctl/state.json` and resumed with `claude --resume` when ccctl starts again.
- **Scrollback.** Each session keeps its last 1 MiB of output in memory and replays it when a browser connects.

## Install

You need Tailscale running on the host and on the viewing device, Claude Code installed and logged in on the host, and git 2.20 or newer.

For the first install, download a release binary. The repo is private, so use `gh`:

```sh
gh release download --repo ryabinski-labs/claude-code-controller --pattern 'ccctl_darwin_arm64.tar.gz'
tar xzf ccctl_darwin_arm64.tar.gz ccctl
./ccctl install          # downloads the latest release into ~/.ccctl/bin and starts the service
~/.ccctl/bin/ccctl status
```

Pick the archive that matches the host: `ccctl_{darwin,linux}_{arm64,amd64}.tar.gz`.

`ccctl install` does the following:

1. Downloads the latest release, or the tag given with `--version vX.Y.Z`, using `$GITHUB_TOKEN` or `gh auth token`. With `--local`, it copies the running binary instead of downloading.
2. Records the absolute path of `claude` from your shell as `claude_path`. launchd and systemd do not load your shell `PATH`.
3. Writes the service and starts it:
   - **macOS:** a launchd user agent `~/Library/LaunchAgents/com.ryabinski-labs.ccctl.plist` with `KeepAlive`.
   - **Linux:** a systemd user unit `~/.config/systemd/user/ccctl.service` with `Restart=always`. Run `loginctl enable-linger $USER` to keep it running while you are logged out.

The service gets your install-time `PATH`, so Claude's tools find `git`, `go`, `node`, and the rest.

To roll back, run `ccctl install --version <previous tag>`. `ccctl uninstall` removes the service only. It keeps `~/.ccctl` and every worktree.

**macOS privacy prompt:** the first time the launchd agent scans `~/Documents` for repos, macOS asks whether `ccctl` may access Documents. The dialog shows on the host's own screen (use Screen Sharing on a headless Mac). Click Allow, or scanning `~/Documents` finds nothing.

## Commands

| Command | Purpose |
|---|---|
| `ccctl serve` | Run the controller in the foreground (the service runs this). |
| `ccctl status` | Print the controller URL `http://TAILSCALE_IP:PORT`. Exits 3 when Tailscale is not running. |
| `ccctl install [--version vX.Y.Z] [--local]` | Install or upgrade the binary and the background service. |
| `ccctl uninstall` | Stop and remove the background service. |
| `ccctl hook --slot N` | Called by Claude Code hooks. Not for manual use. |

Exit codes: 0 success, 1 runtime error, 2 usage error, 3 Tailscale not running.

## Configuration: `~/.ccctl/config.toml` (mode 0600)

| Key | Default | Meaning |
|---|---|---|
| `roots` | `["~/projects", "~/Documents"]` | Folders scanned up to 3 levels deep for git repos. Missing roots are skipped. Direct children of `~` are always checked. |
| `port` | `7681` | HTTP port, bound only on the host's Tailscale IPv4. |
| `allowed_logins` | the host node's Tailscale login | Tailscale logins allowed to use the controller. |
| `claude_path` | recorded by `ccctl install` | Absolute path of the `claude` binary. |
| `tailscale_path` | `tailscale` on PATH, else `/Applications/Tailscale.app/Contents/MacOS/Tailscale` | Tailscale CLI. |

State lives in `~/.ccctl/state.json` (0600). Logs are JSON lines in `~/.ccctl/logs/`, rotated at 10 MB with 7 files kept. Terminal output is never written to disk.

## Security notes

- **Every session runs with `--dangerously-skip-permissions`.** Anything a session decides to run executes as the host user, without asking. Worktrees isolate the files each task edits, but not the machine.
- **Plain HTTP on the Tailscale IP.** WireGuard encrypts the network path, but there is no TLS.
- **Access checks.** Every HTTP request and WebSocket upgrade is checked against `allowed_logins` with Tailscale WhoIs. Anyone else gets HTTP 403, and the refusal is logged as `auth_denied`. WebSocket upgrades must also carry the controller's own `Origin`, and requests must name the controller's own host.
- **Tailscale down.** When Tailscale stops, the controller stops listening, keeps your sessions running, and starts listening again when Tailscale returns.

## Development

```sh
npm --prefix web install
npm --prefix web run dev         # Vite dev server for the frontend
npm --prefix web run build       # builds web/dist, which the Go binary embeds
go run ./cmd/ccctl serve         # needs Tailscale running on this machine
```

## Tests

```sh
go vet ./... && go test -race ./...   # Go unit + integration (fake claude, fake Tailscale, real PTYs and git)
npm --prefix web test                 # Vitest
npm --prefix web run e2e              # Playwright, headless Chromium only
```

Test names carry scenario IDs (`SC-xxx-y`) from the TDD artifact. The manual scenarios (latency over the tailnet, install on mac-mini, crash recovery, memory, first load) are run by hand on the host.

## Release

CI runs on the org's self-hosted Linux runners. Each merge to `main` tags `v0.1.<run>` and publishes darwin and linux binaries for arm64 and amd64 with GoReleaser. Releases are never deleted.
