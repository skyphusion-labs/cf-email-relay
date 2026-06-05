# Integrating callers

Two ways to reach the worker. Same-account Workers use the service binding (no
token, no public hop). Everything else uses the public HTTPS endpoint with the
shared `RELAY_TOKEN`.

## Another Worker (service binding)

Add a service binding pointing at the `EmailService` RPC entrypoint:

```jsonc
// caller wrangler.jsonc
{
  "services": [
    {
      "binding": "EMAIL",
      "service": "cf-email-relay",
      "entrypoint": "EmailService"
    }
  ]
}
```

After editing the binding, regenerate types: `npx wrangler types`.

Then send from anywhere in the worker:

```typescript
const { messageId } = await env.EMAIL.send({
  to: "user@example.com",
  subject: "Welcome",
  html: "<p>Thanks for signing up.</p>",
  text: "Thanks for signing up.",
  // from defaults to DEFAULT_FROM; override with any allowed sender:
  // from: { email: "support@example.com", name: "Support" },
});
```

`send()` throws on failure; the thrown error carries `.code` (an `E_*` string)
and `.message`. Wrap in try/catch and log the code.

## External callers (public HTTPS endpoint)

`POST https://cf-email-relay.<account>.workers.dev/send` with
`Authorization: Bearer <RELAY_TOKEN>`. Body is the same shape as `send()`:

```bash
curl https://cf-email-relay.<account>.workers.dev/send \
  -H "Authorization: Bearer $RELAY_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "to": "user@example.com",
    "subject": "Hello",
    "text": "Hello from cf-email-relay."
  }'
```

Responses:

| Status | Body | Meaning |
|--------|------|---------|
| 200 | `{"ok":true,"messageId":"..."}` | Sent |
| 400 | `{"ok":false,"error":"E_...","message":"..."}` | Bad request, do not retry |
| 401 | `{"ok":false,"error":"unauthorized"}` | Missing/wrong bearer token |
| 502 | `{"ok":false,"error":"E_...","message":"..."}` | Transient upstream, retry with backoff |

## Request fields

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `to` | string \| string[] | yes | Recipient(s) |
| `subject` | string | yes | |
| `html` / `text` | string | one required | Include both for deliverability |
| `from` | string \| `{email,name}` | no | Defaults to `DEFAULT_FROM`; must be on `ALLOWED_FROM_DOMAIN` if that is set |
| `replyTo` | string \| `{email,name}` | no | |
| `cc` / `bcc` | string \| string[] | no | to+cc+bcc <= 50 |
| `headers` | object | no | Whitelisted headers only |
