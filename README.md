# truemail-go

Self-hosted email validation HTTP service in Go. Checks an address without sending mail: format, disposable-domain and role-account detection, DNS/MX lookup, and optional SMTP and IMAP probes. Returns a status plus a 0–100 confidence score.

No API keys, no per-lookup billing, no third-party service — it talks to the recipient's mail servers directly.

## Two services

| Service | Lines | What it is |
|---|---|---|
| [`truemail-go-service/`](truemail-go-service/) | 520 | The stable service. Per-address rate limiting, configurable bulk concurrency. Start here. |
| [`tm2-service/`](tm2-service/) | 733 | Experimental v2: connection pooling, worker pool, batched bulk. Same API surface. Not as well exercised. |

Both expose the same three endpoints and can be run interchangeably.

## API

### `GET /health`

```json
{
  "status": "healthy",
  "service": "truemail-go-real",
  "timestamp": "2026-08-05T18:31:49Z",
  "note": "Using real SMTP + IMAP validation with rate limiting"
}
```

### `POST /validate`

```bash
curl -X POST localhost:8080/validate \
  -H 'Content-Type: application/json' \
  -d '{"email":"someone@example.com"}'
```

```json
{
  "email": "someone@example.com",
  "status": "valid",
  "score": 90,
  "details": "Email validated with SMTP check",
  "validated_at": "2026-08-05T18:00:00Z"
}
```

### `POST /validate/bulk`

```bash
curl -X POST localhost:8080/validate/bulk \
  -H 'Content-Type: application/json' \
  -d '{"emails":["a@example.com","b@example.com"],"fast":true}'
```

Returns `{"results":[ ... ]}`. Set `"fast": true` to skip SMTP probing for the batch.

## Status and score

`status` is one of `valid`, `invalid`, `unknown`, or `rate_limited`. The score reflects how much of the check chain passed:

| Score | Status | Meaning |
|---|---|---|
| 100 | `valid` | SMTP **and** IMAP checks passed |
| 90 | `valid` | SMTP check passed |
| 70 | `valid` | Role-based address (`info@`, `admin@`) — real, but lower deliverability |
| 60 | `unknown` | Domain and MX valid, SMTP probe failed |
| 40 | `unknown` | Domain valid, no MX records |
| 0 | `invalid` | Bad format, disposable domain, or no DNS |
| 0 | `rate_limited` | Same address queried too soon — retry later |

`unknown` is a real answer, not a failure: many hosts (notably Microsoft 365) accept every RCPT TO, so a failed SMTP probe does not prove an address is dead. Treat `unknown` as "do not discard."

## Configuration

All via environment variables:

| Variable | Default | Effect |
|---|---|---|
| `PORT` | `8080` | Listen port |
| `ENABLE_SMTP_VALIDATION` | off | Set `true` to enable SMTP/IMAP probing. Off = DNS/MX only, much faster |
| `BULK_VALIDATION_CONCURRENCY` | built-in | Parallel validations per bulk request |
| `RATE_LIMIT_SECONDS` | built-in | Per-address cooldown. `0` disables rate limiting |

## Run

```bash
cd truemail-go-service && go run .
```

Docker:

```bash
cd truemail-go-service
docker build -t truemail-go .
docker run -p 8080:8080 -e ENABLE_SMTP_VALIDATION=true truemail-go
```

## Notes on SMTP probing

- **Port 25 is usually blocked** on residential ISPs and most cloud providers (AWS, GCP, Azure, DigitalOcean block it by default). Without outbound 25 the SMTP probe always fails and you get `unknown` results. Run this somewhere with port 25 egress, or leave `ENABLE_SMTP_VALIDATION` off.
- The probe issues `MAIL FROM:<validation@example.com>` and never sends a message.
- Probing at volume from one IP gets that IP greylisted or blocklisted. Keep the rate limit on and concurrency modest.
- Verify addresses you have a lawful basis to process. List hygiene on your own opt-in data is fine; scanning lists you acquired elsewhere may not be.

## License

MIT — see [LICENSE](LICENSE).
