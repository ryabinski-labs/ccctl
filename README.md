# ccctl

**ccctl is a web controller for up to six live [Claude Code](https://docs.claude.com/en/docs/claude-code) sessions on one machine, reached privately over Tailscale.**

> Unofficial community project. It is not affiliated with, endorsed by, or sponsored by Anthropic. "Claude" and "Claude Code" are trademarks of Anthropic.

ccctl is built for developers who run several Claude Code sessions in parallel on an always-on Mac or Linux host. Open `http://TAILSCALE_IP:7681/` from any device on your tailnet and get a grid of real `claude` terminals that grows from one pane to six as you start sessions. The problem it solves: a session blocked on a prompt in a terminal you are not looking at. You watch them all, type into any of them, and see at a glance which one is waiting for you.

## At a glance

| | |
|---|---|
| **What it is** | One Go binary (`ccctl`) with an embedded Vue 3 + xterm.js web UI, run as a background service on the host that runs Claude Code. |
| **Who it is for** | People who run several Claude Code sessions in parallel on a Mac or Linux box (a Mac mini, a home server) and want to check on them from a laptop or desktop browser (the page needs a window at least 1024 px wide; phone layouts are not supported). |
| **Problem it solves** | Parallel agent sessions sit blocked on a prompt in a terminal you are not looking at. ccctl shows all of them in one page and flags the one that needs you: an amber pane header, a tab-title count, and an optional chime. |
| **Outcome** | Start, watch, and answer up to six sessions from any device on your tailnet, with each task isolated in its own git worktree. |
| **Status** | Early (`v0.1.x`). Used daily by its author. Expect rough edges and breaking changes. |
| **Platforms** | Host: macOS or Linux, arm64 or amd64. Viewer: any modern browser on your tailnet. |
| **License** | [MIT](LICENSE) |

### Use it when

- Claude Code is installed and logged in on one always-on machine, and you want to drive it from other devices.
- You already use Tailscale, and "reachable only on my tailnet" is the access model you want.

### Do not use it when

- You need to expose sessions to the public internet, other tailnets, or people you do not trust. Every session runs with `--dangerously-skip-permissions`, so anyone with access controls your machine. See [Security notes](#security-notes).
- You need TLS, per-session permissions, or more than six sessions. None of these exist.
- You want a hosted service or multi-user product. ccctl is a single-user tool.

## Compared with

All of these let you reach a Claude Code session from another device. ccctl's niche is a single self-hosted page that shows several live terminals at once and flags the one that needs you. Details below were checked on 2026-10-01 and change often; see each project's docs.

| | ccctl | Claude Code [Remote Control](https://code.claude.com/docs/en/remote-control) | Browser UIs such as [CloudCLI](https://github.com/siteboon/claudecodeui) | `tmux` over SSH |
|---|---|---|---|---|
| Where sessions run | Your machine | Your machine | Your machine, or a hosted tier | Your machine |
| Path to your sessions | Direct, over your tailnet | Through Anthropic's servers; the transcript is stored there | Direct (self-hosted) | Direct, over SSH |
| Sign-in needed | Your Tailscale login | A claude.ai subscription (API keys are not supported) | None beyond the tool's own | SSH key |
| Several sessions at a glance | Grid of live terminals (1 to 6, resizes as you add) | Session list in the app or claude.ai/code | Session list and tabs | Panes, if you set them up |
| "Needs input" signal | Amber pane header, tab count, chime | Mobile push notifications | Not documented | None |
| Worktree per task | Yes, by default for git repos | Optional (`--spawn worktree`) | Not documented | Manual |
| Cost | Free, MIT | Included with a Claude subscription | Free (AGPL); hosted tier is paid | Free |

Pick Remote Control if you want Anthropic's mobile app and push notifications, and are happy with a claude.ai login. Pick ccctl if you want the traffic to stay on your own tailnet, work with API-key auth, or watch several terminals side by side.

## Quickstart

On the host (the machine that runs Claude Code), with Tailscale running and `claude` logged in:

```sh
# Pick your platform: ccctl_{darwin,linux}_{arm64,amd64}.tar.gz
curl -fsSL -O https://github.com/ryabinski-labs/ccctl/releases/latest/download/ccctl_darwin_arm64.tar.gz
tar xzf ccctl_darwin_arm64.tar.gz ccctl
./ccctl install
~/.ccctl/bin/ccctl status
```

Expected result: `ccctl status` prints the controller URL, for example `http://100.64.0.10:7681`. Open it from any device on your tailnet. On macOS, read [macOS firewall](#macos-firewall) if the page never loads.

## Documentation

- **Spec:** `docs/prd/claude-code-controller.md` (a historical design record)
- **TDD artifact:** `tdd/claude-code-controller.tdd.yaml`
- **UI brief:** `docs/design/ui-brief.md`
- **Wire protocol:** `docs/design/protocol.md`

## How it works

- **One binary.** `ccctl` is a Go program with the Vue 3 + xterm.js frontend embedded in it.
- **Sessions.** Each session is `claude --dangerously-skip-permissions --session-id UUID --settings FILE [prompt]` running in its own PTY. For git repos, each task gets its own worktree at `<repo>-wt-<task>` on branch `ctl/<task>`, created from HEAD. Worktrees and branches are never removed by ccctl.
- **Needs input.** Claude Code's `Notification` and `Stop` hooks call `ccctl hook --slot N` over `~/.ccctl/ccctl.sock`. The pane header turns amber, the tab title shows a count, and a chime plays (you can mute it).
- **Files and images.** Paste a file or screenshot onto a pane, drop one on it, or use the paperclip button in the pane header. ccctl saves it on the host in `~/.ccctl/uploads` (up to 25 MB each; ccctl removes its own uploads after 14 days and refuses new ones once the folder passes 1 GB) and types its path into the session, which is how Claude Code attaches an image. Pasting text works as in any terminal.
- **Restarts.** Live sessions are recorded in `~/.ccctl/state.json` and resumed with `claude --resume` when ccctl starts again.
- **Scrollback.** Each session keeps its last 1 MiB of output in memory and replays it when a browser connects.

## Install

### Requirements

Tailscale running on the host and on the viewing device, Claude Code installed and logged in on the host, and git 2.20 or newer. Linux hosts need systemd user services.

### Install

Download a release binary as shown in the [Quickstart](#quickstart), then run `ccctl install`. To check the download first, compare it against `checksums.txt` on the release page, or verify its build provenance with `gh attestation verify ccctl_darwin_arm64.tar.gz --repo ryabinski-labs/ccctl`.

The macOS binaries are not signed or notarized. Downloading with `curl`, as above, avoids the Gatekeeper quarantine prompt that a browser download triggers.

`ccctl install` does the following:

1. Downloads the latest release, or the tag given with `--version vX.Y.Z`, anonymously, or with `$GITHUB_TOKEN` or `gh auth token` when one is available (this avoids GitHub's lower anonymous rate limit). With `--local`, it copies the running binary instead of downloading.
2. Records the absolute path of `claude` from your shell as `claude_path`. launchd and systemd do not load your shell `PATH`.
3. Writes the service and starts it:
   - **macOS:** a launchd user agent `~/Library/LaunchAgents/com.ryabinski-labs.ccctl.plist` with `KeepAlive`.
   - **Linux:** a systemd user unit `~/.config/systemd/user/ccctl.service` with `Restart=always`. Run `loginctl enable-linger $USER` to keep it running while you are logged out.

The service gets your install-time `PATH`, so Claude's tools find `git`, `go`, `node`, and the rest.

To roll back, run `ccctl install --version <previous tag>`. `ccctl uninstall` removes the service only. It keeps `~/.ccctl` and every worktree.

**Repository folder:** nothing is scanned until you set one. Open Settings, enter the folder that holds your repos (for example `~/projects`), and press Save folder. New session then lists only the git repos under it. You can always type any folder with "Use a custom path".

**macOS privacy prompt:** the first time the launchd agent scans a folder under `~/Documents`, `~/Desktop` or `~/Downloads`, macOS asks whether `ccctl` may access it. The dialog shows on the host's own screen (use Screen Sharing on a headless Mac). Click Allow. Until then the scan waits, and after 10 seconds New session says so and offers Retry.

### Environment for sessions (API keys)

Sessions run under launchd or systemd, so they do not see your shell profile. Put variables every
Claude session should get in an `[env]` table in `~/.ccctl/config.toml` (the file is mode 0600):

```toml
[env]
GEMINI_API_KEY = "..."
```

The table is re-read at every launch, so an edit applies to the next session you start, with no
restart. Sessions that are already running keep the environment they started with; stop and relaunch one
to pick up a change. Values are never written to the log. `CCCTL_*` and `TERM` cannot be overridden.

### macOS firewall

If the macOS Application Firewall is on, it silently holds connections to a newly
installed or upgraded `ccctl` binary (the page never loads; `ccctl status` still says it is serving).
`ccctl install` prints the command; run it once per install or upgrade on the host:

```sh
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --add ~/.ccctl/bin/ccctl
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --unblockapp ~/.ccctl/bin/ccctl
```

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
| `repo_prefix` | unset (no scan) | The one folder New session scans, up to 3 levels deep, for git repos. Set it in Settings or here. A `roots` key from an older version is ignored. |
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

Test names carry scenario IDs (`SC-xxx-y`) from the TDD artifact. The manual scenarios (latency over the tailnet, install on a real host, crash recovery, memory, first load) are run by hand on the host.

## Release

CI runs on GitHub-hosted Linux runners. Each merge to `main` tags `v0.1.<run>` and publishes darwin and linux binaries for arm64 and amd64 with GoReleaser, plus `checksums.txt` and a build-provenance attestation. Releases are never deleted.

## Contributing, support, security

- **Contribute:** see [CONTRIBUTING.md](CONTRIBUTING.md). Bug reports and focused pull requests are welcome.
- **Get help:** see [SUPPORT.md](SUPPORT.md). This is a volunteer project with no response-time guarantee.
- **Report a vulnerability:** privately, as described in [SECURITY.md](SECURITY.md).
- **Maintainers:** [MAINTAINERS.md](MAINTAINERS.md). **Conduct:** [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## License

[MIT](LICENSE) © 2026 Yoni Ryabinski. Third-party components keep their own licenses; see their packages.
