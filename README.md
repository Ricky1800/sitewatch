# sitewatch

A single-binary uptime, SSL-expiry, and content monitor for small-business
websites. No database, no SaaS subscription, no dashboard to host — just a
YAML config, a binary, and (optionally) a static HTML status page you can
drop anywhere.

## The problem

A small business's website usually has no one watching it. The freelancer
or agency who built it moves on to the next project, the hosting renews
automatically, and the TLS certificate quietly expires eight months later —
or the server starts returning 500s at 2am on a Saturday — and nobody knows
until a customer complains, or worse, doesn't complain and just leaves.
Commercial uptime monitors exist, but they're one more recurring bill for a
business that's already juggling a dozen of them, and they're overkill for
"tell me if my one website goes down."

sitewatch is built for the person maintaining a handful of small-business
sites (an agency, a freelancer, or the business owner themselves): one
binary, one config file, checks every site's HTTP status/content/response
time/certificate expiry, and alerts wherever you already look (Discord,
Slack, a webhook into your own tooling, or plain email) the moment
something changes.

## Quickstart

```sh
# Build from source (Go 1.22+)
git clone https://github.com/Ricky1800/sitewatch.git
cd sitewatch
go build -o sitewatch .

# Copy and edit the example config
cp examples/sitewatch.yaml sitewatch.yaml
# ... edit sitewatch.yaml: your checks, your alert webhooks ...

# One-shot check (good for a first test, or for CI/cron)
./sitewatch check

# Start the long-running monitor
./sitewatch run
```

`sitewatch check` exits `1` if any check fails, so it also works as a CI
step or a cron job that alerts through your existing monitoring (e.g. a
cron wrapper that emails on non-zero exit).

## Commands

| Command | Purpose |
|---|---|
| `sitewatch run [--config sitewatch.yaml]` | Start the daemon. Runs until SIGINT/SIGTERM. |
| `sitewatch check [--config sitewatch.yaml] [--json]` | Run every check once, print results, exit 1 on any failure. |
| `sitewatch status [--config sitewatch.yaml] [--out status.html]` | Generate a static HTML status page from recorded history. |
| `sitewatch version` | Print the version. |

## Config reference

`sitewatch.yaml`:

```yaml
defaults:
  interval: 60s          # how often to check, per-check overridable
  timeout: 10s            # per-request timeout, per-check overridable
  fail_threshold: 3       # consecutive failures before a DOWN alert fires
  ssl_warn_days: 14       # warn once/day once the cert is this close to expiry

alerts:
  discord:
    webhook_url: "https://discord.com/api/webhooks/..."
  slack:
    webhook_url: "https://hooks.slack.com/services/..."
  webhook:
    url: "https://your-own-endpoint.example.com/hooks/sitewatch"
    headers:
      X-Api-Key: "shared-secret"
  email:
    smtp_host: "smtp.gmail.com"
    smtp_port: 587
    username: "alerts@example.com"
    from: "alerts@example.com"
    to: ["oncall@example.com"]
    password_env: "SITEWATCH_SMTP_PASSWORD"   # read at runtime; never in this file

history:
  path: "sitewatch_history.jsonl"
  max_age_days: 90

checks:
  - name: "Acme Plumbing — Homepage"    # required, must be unique
    url: "https://acmeplumbing.example.com"  # required, http:// or https://
    interval: 60s                        # optional, falls back to defaults.interval
    timeout: 10s                         # optional
    expected_status: 200                 # optional, default 200
    body_contains: "Acme Plumbing"        # optional substring the response must contain
    max_response_ms: 3000                # optional response-time budget
    ssl_warn_days: 14                    # optional
    fail_threshold: 3                    # optional
```

Every duration field accepts Go duration syntax (`"30s"`, `"5m"`,
`"1h30m"`). See `examples/sitewatch.yaml` for a complete, commented example.

Config errors are reported with the offending YAML line number where one
applies, e.g.:

```
config validation failed (2 error(s)):
  - line 4: checks[0].url: is required
  - line 9: alerts.email.password_env: is required; sitewatch never stores SMTP passwords in the config file, so name the environment variable to read it from
```

### Alerting behavior (no flapping spam)

sitewatch only alerts on a *state change*, not on every failed probe:

- **Down**: fires once, the moment consecutive failures reach
  `fail_threshold`. Further failures while already down do not re-alert.
- **Up**: fires once, on the first success after being down, reporting how
  long the check was down.
- **SSL expiry**: fires at most once per calendar day per check, while the
  certificate is inside `ssl_warn_days`.

## Setting up alerts

**Discord** — Server Settings → Integrations → Webhooks → New Webhook, copy
the URL into `alerts.discord.webhook_url`.

**Slack** — create an [Incoming Webhook](https://api.slack.com/messaging/webhooks)
for the channel you want alerts in, copy the URL into
`alerts.slack.webhook_url`.

**Generic webhook** — point `alerts.webhook.url` at your own endpoint (an
n8n/Zapier flow, an internal tool, a PagerDuty routing endpoint via a small
adapter, etc). sitewatch POSTs a small JSON body:
`{"event": "DOWN", "check": "...", "url": "...", "time": "...", "message": "...", ...}`.
Add any headers it needs (auth tokens, etc) under `alerts.webhook.headers`.

**Email** — set `smtp_host`/`smtp_port`/`username`/`from`/`to` in
`alerts.email`, then set the environment variable named by `password_env`
before starting sitewatch:

```sh
export SITEWATCH_SMTP_PASSWORD="an app password, not your account password"
./sitewatch run
```

Gmail and most providers require an **app password**, not your normal
account password, once 2FA is enabled. sitewatch never reads a password
directly from the config file — only the name of the environment variable
to read it from — so `sitewatch.yaml` is safe to commit to a repo (as long
as the webhook URLs in it aren't sensitive to you; treat them as secrets
too if that matters for your setup).

## Status page

```sh
./sitewatch status --out status.html
```

Produces a single self-contained HTML file (no external CSS/JS/fonts —
safe to open offline or serve from anywhere) showing, per check: current
state (a colored UP/DOWN/UNKNOWN badge), last-checked time, 24h/7d/30d
uptime percentages, and an inline SVG sparkline of recent response times.
Run it on a schedule (cron, or a CI job) and publish the output to your
static host of choice (GitHub Pages, S3, Netlify, anywhere).

The page renders a table: check name and URL, a status badge, "last
checked", the three uptime columns, and a small inline line chart of
response times — light/dark aware via `prefers-color-scheme`, and legible
at phone width.

## Running it

### As a systemd service

```sh
sudo cp sitewatch /usr/local/bin/
sudo mkdir -p /etc/sitewatch
sudo cp sitewatch.yaml /etc/sitewatch/
sudo cp examples/sitewatch.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now sitewatch
```

Put secrets (like `SITEWATCH_SMTP_PASSWORD`) in
`/etc/sitewatch/sitewatch.env` (referenced by the unit file's
`EnvironmentFile=`), not in the unit file itself.

### With Docker

```sh
docker build -t sitewatch .
docker run -d --name sitewatch \
  -v "$(pwd)/sitewatch.yaml:/etc/sitewatch/sitewatch.yaml" \
  -v sitewatch-data:/etc/sitewatch \
  -e SITEWATCH_SMTP_PASSWORD \
  sitewatch
```

### With cron (one-shot mode instead of the daemon)

```cron
*/5 * * * * cd /etc/sitewatch && ./sitewatch check >> /var/log/sitewatch.log 2>&1
0 6 * * *   cd /etc/sitewatch && ./sitewatch status --out /var/www/status/index.html
```

## Development

```sh
go build ./...
go test ./...
go vet ./...
gofmt -l .          # should print nothing
go test -race ./...  # requires cgo; not run on Windows in CI for that reason
```

Tests use `net/http/httptest` (including `httptest.NewTLSServer` for the
certificate-expiry logic) and a fake clock (`internal/clock`) so the
state-machine, uptime-math, and alert-payload tests run fast and
deterministically, with no real sleeps and no real network calls except in
the small handful of tests that specifically exercise HTTP behavior against
a local test server.

## Roadmap

See [`ROADMAP_ISSUES.md`](ROADMAP_ISSUES.md) for scoped, ready-to-pick-up
future work (Telegram/SMS/PagerDuty alert channels, a DNS check type, regex
body matching, maintenance windows, a Prometheus metrics endpoint).

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md).

## Authors

- [@Ricky1800](https://github.com/Ricky1800)
- [@orbitwebsites-cloud](https://github.com/orbitwebsites-cloud) ([OrbitBoyzz](https://orbitboyzz.me))

## License

[MIT](LICENSE)
