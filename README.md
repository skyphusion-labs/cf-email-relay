# cf-email-relay

[![ci](https://github.com/SkyPhusion/cf-email-relay/actions/workflows/ci.yml/badge.svg)](https://github.com/SkyPhusion/cf-email-relay/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Send transactional email through [Cloudflare Email
Sending](https://developers.cloudflare.com/email-service/) from anywhere, two
small pieces:

- **`worker/`** — a Cloudflare Worker that sends mail via the `send_email`
  binding. It exposes an RPC entrypoint for same-account Workers (service
  binding, no token) and a token-gated public `POST /send` endpoint for
  everything else.
- **`relay/`** — a tiny Go SMTP daemon. Cloudflare Email Sending has no SMTP
  interface, so this bridges it: local services that can only speak SMTP (cron,
  backups, monitoring daemons, appliances) hand it a message and it relays to
  the worker's `/send` over HTTPS.

```
your Worker  ──(service binding RPC: env.EMAIL.send)──┐
                                                       ├──► worker ──► CF Email Sending ──► inbox
SMTP-only services ──SMTP──► relay ──(HTTPS + Bearer)──┘
  (cron, backups,            (127.0.0.1:2525,
   monitoring, etc.)          systemd on the box)
```

Why the split: same-account Workers get a typed, tokenless, no-network-hop RPC
call. Anything that can't be a Worker speaks plain SMTP to a localhost relay and
never has to learn the HTTP API.

## Worker

### Prerequisites (once)

Onboard a sending domain so SPF/DKIM records are added to Cloudflare DNS:

```bash
cd worker
npx wrangler email sending enable yourdomain.com
npx wrangler email sending dns get yourdomain.com   # verify records landed
```

### Configure

Edit `worker/wrangler.jsonc` vars:

- `DEFAULT_FROM` — address used when a request omits `from` (e.g.
  `noreply@yourdomain.com`).
- `DEFAULT_FROM_NAME` — optional display name.
- `ALLOWED_FROM_DOMAIN` — optional. If set, only From addresses on this domain
  are accepted. Leave empty to allow any onboarded sender domain.

### Deploy

```bash
cd worker
npm install
npx wrangler secret put RELAY_TOKEN     # shared secret for the public endpoint
npm run deploy
```

Generate a strong token with `openssl rand -hex 32`. The same value goes into
the relay's `EMAIL_RELAY_TOKEN`. `RELAY_TOKEN` is a secret and is never
committed.

### Endpoints

- `GET /` or `/health` — liveness, no auth.
- `POST /send` — send mail. Requires `Authorization: Bearer <RELAY_TOKEN>`.

See [docs/INTEGRATION.md](docs/INTEGRATION.md) for the service binding setup, the
request schema, and response/error codes.

## Relay (optional)

Only needed if you have SMTP-only services. Build it with Go >= 1.22:

```bash
cd relay
go build -o cf-email-relay .
```

Install (systemd):

```bash
sudo install -m 0755 cf-email-relay /usr/local/bin/
sudo install -m 0600 cf-email-relay.env.example /etc/cf-email-relay.env
sudoedit /etc/cf-email-relay.env        # set EMAIL_WORKER_URL + EMAIL_RELAY_TOKEN
sudo install -m 0644 systemd/cf-email-relay.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now cf-email-relay
```

Point local services at `127.0.0.1:2525` (no auth, loopback only). Quick test
with [swaks](https://github.com/jetmore/swaks):

```bash
swaks --server 127.0.0.1:2525 --from cron@yourdomain.com \
      --to you@example.com --header "Subject: relay test" --body "hello"
```

The relay uses the envelope `RCPT TO` for recipients. If `FROM_DOMAIN` is set and
a message's `From` isn't on it, the relay rewrites the From to `DEFAULT_FROM` and
keeps the original as `Reply-To` (handy when the worker restricts the sender
domain). Leave `FROM_DOMAIN` empty to pass the original `From` through.

### Reaching the relay from a container

The relay binds loopback by default. `SMTP_LISTEN` accepts a comma-separated
list, so to also serve a container on a docker bridge, bind that bridge's
gateway IP too (find it with `docker network inspect <net>`):

```
SMTP_LISTEN=127.0.0.1:2525,172.18.0.1:2525
```

Bind specific private IPs, never `0.0.0.0` — the relay sends as your domain, so
an externally reachable port is abusable. If a host firewall is in the way,
allow only that bridge interface to the port.

## Layout

```
worker/
  src/index.ts   RPC entrypoint (EmailService) + public fetch handler
  src/email.ts   validation + the actual env.EMAIL.send() call
  src/env.d.ts   binding/var types
  wrangler.jsonc send_email binding + vars
relay/
  main.go        entrypoint
  config.go      env-driven config
  smtp.go        go-smtp backend, MIME parse, payload build, multi-listen
  client.go      HTTPS POST to the worker
  systemd/       service unit
docs/
  INTEGRATION.md caller setup (service binding + REST)
```

## Contributing

PRs welcome. CI (worker typecheck + relay build) must pass before merge. See
[CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT, see [LICENSE](LICENSE).
