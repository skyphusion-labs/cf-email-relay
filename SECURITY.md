# Security Policy

## Supported versions

This project is pre-1.0. Security fixes land on `main` and the newest `v0.x`
release; older releases are not maintained.

| Version | Supported |
|---------|-----------|
| `main` + newest `v0.x` | yes |
| older | no |

## Reporting a vulnerability

Please report security issues **privately**, not as a public issue or pull
request.

Use GitHub's private vulnerability reporting: open the repository's **Security**
tab and click **Report a vulnerability**, or go directly to
<https://github.com/SkyPhusion/cf-email-relay/security/advisories/new>.

Include enough detail to reproduce: affected component (`worker` / `relay`),
versions, steps, and the impact. You'll get an acknowledgement, and a fix and
coordinated disclosure from there.

## Deployment notes that affect security

- `RELAY_TOKEN` gates the worker's public `POST /send`. Treat it as a secret and
  use a high-entropy value (`openssl rand -hex 32`).
- The SMTP relay sends as your configured domain. Bind it to specific private
  IPs (loopback, or a docker-bridge gateway), **never `0.0.0.0`**, so the SMTP
  port is not reachable from the internet.
