# Contributing to cf-email-relay

Thanks for your interest. This is a small, focused project, so changes that keep
it small and focused are the easiest to land.

## Ground rules

- By contributing, you agree your work is licensed under the repo's
  [MIT license](LICENSE).
- Keep each PR scoped to one change; small PRs get reviewed faster.
- **CI must pass before a PR can be merged.** Every PR runs two checks,
  `worker` and `relay` (see [.github/workflows/ci.yml](.github/workflows/ci.yml)),
  and `main` is protected so they're required.

## Project layout

- `worker/` — the Cloudflare Worker (TypeScript)
- `relay/` — the Go SMTP-to-API relay
- `docs/` — integration docs

## Running the checks locally

Worker (Node 22):

```bash
cd worker
npm ci
npm run typecheck
```

Relay (Go >= 1.22):

```bash
cd relay
go vet ./...
go build ./...
```

Those are exactly the two checks CI runs.

## Style

- Match the surrounding code; mirror its naming and comment density.
- Keep the dependency footprint minimal: the worker has no runtime dependencies,
  and the relay keeps its set small. Please don't add dependencies without a
  clear reason.
- Conventional-commit messages (`feat:`, `fix:`, `docs:`, ...) are appreciated
  but not required.

## Bugs and ideas

Open an issue with enough detail to reproduce: wrangler/Go versions, what you
ran, and what happened.

## Security

Please don't file security issues in public. Use GitHub's **private vulnerability
reporting** (the repo's Security tab > Report a vulnerability) instead.
