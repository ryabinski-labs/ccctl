# Contributing to ccctl

Thanks for helping. Bug reports, fixes, and focused features are welcome. By taking part you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

## Before you start

- For anything bigger than a small fix, open an issue first so we can agree on the approach.
- ccctl is deliberately small: one binary, up to six sessions, Tailscale-only access. Changes that widen the trust boundary (new listeners, auth changes, weaker defaults) get the closest review.
- Report security problems privately: see [SECURITY.md](SECURITY.md).

## Set up

You need Go (the version in `go.mod`), Node 22, git 2.20+, and a Tailscale install if you want to run the server for real. Tests use a fake `claude` and a fake Tailscale, so they need neither.

```sh
npm --prefix web ci
npm --prefix web run build       # the Go binary embeds web/dist
go build ./...
```

## Run the checks

CI runs exactly these. Please run them before opening a pull request:

```sh
npm --prefix web run typecheck
npm --prefix web test
go vet ./... && go test -race ./...
npm --prefix web run e2e         # Playwright, needs `npx playwright install chromium` once
```

Test names carry scenario IDs (`SC-xxx-y`) from `tdd/claude-code-controller.tdd.yaml`. If you change behavior, add or update the scenario and its test.

## Pull requests

1. Fork the repo, branch from `main`, and keep the change focused.
2. Add or update tests, and update the README or `docs/` for user-visible changes.
3. Open a pull request. CI must pass; a maintainer merges it. Merging to `main` publishes a release automatically.
4. Never include secrets, tokens, or real hostnames or IPs in code, tests, docs, or issue text.

## License of contributions

ccctl is under the [MIT License](LICENSE). By submitting a contribution you license it under the same terms (inbound = outbound). There is no CLA and no sign-off requirement.
