#!/usr/bin/env bash
# Rebuild ccctl from this checkout, install it as the background service, and restart it.
# Works on a fresh host and on one that already runs ccctl. Run it on the host itself:
#
#   scripts/rebuild-restart.sh          # build the current checkout
#   scripts/rebuild-restart.sh --pull   # git pull first
#
# Live sessions survive: ccctl records them in ~/.ccctl/state.json and resumes them with
# `claude --resume` when it starts again. Open browser tabs reconnect on their own.
set -euo pipefail

cd "$(dirname "$0")/.."

die() { echo "error: $*" >&2; exit 1; }
step() { printf '\n==> %s\n' "$*"; }

for cmd in git go npm; do
  command -v "$cmd" >/dev/null || die "$cmd is not installed"
done
command -v claude >/dev/null || die "claude is not on PATH; install Claude Code and log in first"
command -v tailscale >/dev/null || [ -x /Applications/Tailscale.app/Contents/MacOS/Tailscale ] ||
  die "Tailscale is not installed"

if [ "${1:-}" = "--pull" ]; then
  step "Pulling latest changes"
  git pull --ff-only
fi

step "Building the web UI"
npm --prefix web ci
npm --prefix web run build

step "Building ccctl"
version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
out="$(mktemp -d)"
trap 'rm -rf "$out"' EXIT
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$out/ccctl" ./cmd/ccctl

step "Installing ccctl $version and loading the service"
# Copies the binary to ~/.ccctl/bin and restarts the launchd agent or systemd unit.
"$out/ccctl" install --local

bin="$HOME/.ccctl/bin/ccctl"

if [ "$(uname -s)" = "Darwin" ]; then
  fw=/usr/libexec/ApplicationFirewall/socketfilterfw
  if "$fw" --getglobalstate 2>/dev/null | grep -q enabled; then
    step "Allowing the new binary through the macOS firewall (asks for your password)"
    sudo "$fw" --add "$bin" >/dev/null
    sudo "$fw" --unblockapp "$bin" >/dev/null
  fi
fi

step "Waiting for ccctl to answer"
for _ in $(seq 1 30); do
  if url="$("$bin" status 2>/dev/null)" && curl -s -o /dev/null --max-time 2 "$url"; then
    echo "ccctl $("$bin" version) is serving at $url"
    exit 0
  fi
  sleep 1
done
echo "ccctl did not answer within 30 s. Check the logs in ~/.ccctl/logs/ and run: $bin status" >&2
exit 1
