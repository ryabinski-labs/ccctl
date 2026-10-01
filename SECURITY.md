# Security policy

## Supported versions

Only the latest release receives security fixes. Releases are tagged `v0.1.N`; upgrade with `ccctl install`.

## Reporting a vulnerability

Please do not open a public issue for a security problem.

Report it privately through GitHub: go to the **Security** tab of this repository and choose **Report a vulnerability**. Include the ccctl version (`ccctl status` prints the URL; the release tag is on the Releases page), your OS, and the steps to reproduce.

This is a volunteer-maintained project. There is no guaranteed response time, but reports are read and acted on as maintainer time allows. Please give a reasonable window to fix an issue before you disclose it publicly.

## What is in scope

ccctl is a remote controller for coding agents, so its trust boundary matters:

- **Access control.** Anything that lets a Tailscale identity outside `allowed_logins` read or drive a session, or bypasses the WhoIs, `Origin`, or `Host` checks.
- **Exposure.** Anything that makes the controller listen on an address other than the host's Tailscale IP.
- **Secrets.** Anything that writes `[env]` values or terminal output to logs, state files, or API responses.
- **Install and update.** Anything that lets `ccctl install` run an unverified binary.

## Known and by design

These are documented in the README and are not vulnerabilities:

- Every session runs `claude --dangerously-skip-permissions`, so a session can run any command as the host user.
- The controller serves plain HTTP. The Tailscale (WireGuard) tunnel encrypts the network path; there is no TLS.
- Anyone in `allowed_logins` has full control of every session.
