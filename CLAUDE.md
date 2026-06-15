# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A two-part transactional email system that lets anything send mail through **Cloudflare Email
Sending**, even services that only speak SMTP. The generic, open-source sibling of
`skyphusion-email` (which is the skyphusion.org-specific deployment of the same design).

Two components, two languages:

- `worker/` -- a Cloudflare Worker (TypeScript) that actually sends the mail via the
  `send_email` binding. Two front doors into the same `sendEmail()` core:
  - **RPC**: same-account Workers call `env.EMAIL.send({...})` through a service binding (typed,
    no token, no network hop). The class is `EmailService extends WorkerEntrypoint<Env>`.
  - **Public HTTPS**: `POST /send` for external callers, gated by a `RELAY_TOKEN` Bearer secret.
- `relay/` -- an optional Go SMTP daemon (`go-smtp` + `enmime`) for legacy/local services that
  can only speak SMTP (cron, backups, monitoring). It accepts mail on loopback, parses the MIME,
  and POSTs it to the worker's `/send` over HTTPS.

```
modern Worker ──(service binding RPC: env.EMAIL.send)──┐
                                                       ├──► worker ──► CF Email Sending ──► inbox
SMTP-only svc ──SMTP──► relay ──(HTTPS + Bearer token)─┘
```

## Commands

### Worker (`worker/`, Node 22)
```bash
npm run dev          # wrangler dev (local)
npm run deploy       # wrangler deploy
npm run typecheck    # tsc --noEmit -- the CI gate; run before pushing
npm run cf-typegen   # wrangler types (regenerate Env types from wrangler.jsonc)
```

First-time setup: `npx wrangler email sending enable <domain>` (onboards the sending domain,
adds SPF/DKIM in Cloudflare DNS) then `npx wrangler secret put RELAY_TOKEN`.

### Relay (`relay/`, Go 1.22+)
```bash
go vet ./...                       # lint (runs in CI)
go build -o cf-email-relay .       # build (runs in CI)
```
Install on the host: copy the binary to `/usr/local/bin/`, the env file to
`/etc/cf-email-relay.env` (mode 0600), and the unit to `/etc/systemd/system/`, then
`systemctl enable --now cf-email-relay`. See `README.md` for the full sequence.

There is **no automated test suite**. Verify the worker with `npm run dev`, the relay with
`swaks --server 127.0.0.1:2525 ...`, and the HTTP path with `curl .../send`.

## Architecture

Both front doors funnel through one function so behavior can't drift:

- `worker/src/index.ts` -- the dual entry point: `EmailService` (RPC) + the `fetch` handler
  (`GET /health`, `POST /send`). The `/send` path does a **constant-time** Bearer-token compare
  before parsing the body. Keep it constant-time; do not replace with `===`.
- `worker/src/email.ts` -- the shared core: `EmailRequest`, `sendEmail(env, req)`, the
  `EmailError` class (carries `.code` + `.status`). Validates fields/recipients, enforces the
  sender domain, builds the `SendEmailMessage`, calls `env.EMAIL.send()`, maps upstream failures
  (retryable -> 502, caller-fixable -> 400/403).
- `worker/src/env.d.ts` -- hand-authored `Env`, `SendEmailMessage`, `EmailSendBinding` types.
- `relay/config.go` -- env-driven config (no flags, no config file; built for a systemd
  `EnvironmentFile`).
- `relay/smtp.go` -- the `go-smtp` Backend/Session, MIME parse (`enmime`), payload build,
  multi-listen. Recipients come from the **envelope** (RCPT TO), not headers.
- `relay/client.go` -- the HTTPS POST to the worker's `/send` with the Bearer token.

### Sender-domain rewriting (load-bearing)
The worker only accepts `from` addresses on `ALLOWED_FROM_DOMAIN`. When the relay's
`FROM_DOMAIN` is set and an incoming message's `From` is off-domain, the relay rewrites `From`
to `DEFAULT_FROM` and moves the original into `Reply-To` so it does not get rejected. If
`FROM_DOMAIN` is empty, the original `From` passes through unchanged.

## Bindings, vars, secrets

**Worker** (`worker/wrangler.jsonc`, `compatibility_date 2025-05-05`, `observability.enabled`):
- Binding `send_email` -> `EMAIL` (Cloudflare Email Sending).
- Vars: `DEFAULT_FROM`, `DEFAULT_FROM_NAME` (optional), `ALLOWED_FROM_DOMAIN` (optional; empty =
  allow any onboarded sender).
- Secret: `RELAY_TOKEN` (`wrangler secret put`; generate with `openssl rand -hex 32`).
- No D1/R2/KV -- the worker is stateless and has zero runtime deps.

**Relay** (`/etc/cf-email-relay.env`): `EMAIL_WORKER_URL` (required), `EMAIL_RELAY_TOKEN`
(required; must match the worker's `RELAY_TOKEN`), `SMTP_LISTEN` (default `127.0.0.1:2525`,
comma-separated for multiple binds), `DEFAULT_FROM`, `FROM_DOMAIN`, `HTTP_TIMEOUT_SECONDS`
(default 30), `MAX_MESSAGE_BYTES` (default 25 MiB).

## Gotchas
- **Never bind the relay to `0.0.0.0`.** It sends as your domain, so an internet-reachable SMTP
  port is an open spam relay. Loopback or a private bridge IP (e.g. `172.18.0.1`) only.
- **Domain onboarding is the real gate.** `wrangler email sending enable <domain>` must run once
  per sending domain; `ALLOWED_FROM_DOMAIN` is just an extra policy layer on top.
- **Max 50 recipients** (to + cc + bcc), enforced in both `email.ts` (`MAX_RECIPIENTS`) and
  `smtp.go` (`MaxRecipients`). Keep the two in sync if you change it.
- **No queue.** Sends are synchronous; on worker failure the relay returns SMTP 451 (transient)
  so the sending MTA can retry, but nothing is durably buffered.
- Address validation is deliberately loose (`^[^@\s]+@[^@\s]+\.[^@\s]+$`); Cloudflare Email
  Sending does the real validation, so fail fast with a clear code and let the service decide.

## CI / deploy
GitHub Actions (`.github/workflows/ci.yml`) on every push to `main` and all PRs: worker job
(Node 22) runs `npm ci` + `npm run typecheck`; relay job (Go 1.22) runs `go vet` + `go build`.
Both gate the branch. **No auto-deploy** -- the worker ships with `npm run deploy`, the relay is
rebuilt and reinstalled on its host by hand. Served on the assigned `workers.dev` subdomain (no
custom domain).

## Conventions (SkyPhusion house style)
- Default handle/username for any service is `skyphusion`.
- No em-dashes (U+2014) or en-dashes (U+2013) in source, comments, or docs; use commas,
  semicolons, or parentheses.
- `npm run typecheck` must pass before pushing (it is not part of any test run).
- Conventional Commits: `feat(worker): ...`, `fix(relay): ...`, `ci: ...`, `docs: ...`. Body is
  the why; footer lists files touched.
- Keep both components dependency-light (worker: zero runtime deps; relay: only `go-smtp` +
  `enmime`). New deps need justification.
