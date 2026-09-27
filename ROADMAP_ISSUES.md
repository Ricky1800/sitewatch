# Roadmap / good first issues

These are scoped, self-contained pieces of future work — not implemented
yet, but designed to slot cleanly into the existing architecture (config →
checker → state machine → alert.Notifier → history). Each is written so it
could be pasted directly into a GitHub issue.

---

## 1. Telegram alert channel

**Labels:** `good first issue`, `help wanted`

**Body:**

Add a Telegram bot alert channel alongside Discord/Slack/webhook/email.

**Proposed shape** (`alerts.telegram` in `sitewatch.yaml`):

```yaml
alerts:
  telegram:
    bot_token_env: "SITEWATCH_TELEGRAM_BOT_TOKEN"  # never store the token in the file
    chat_id: "-100123456789"
```

Implementation follows the existing `internal/alert/discord.go` pattern:
a `BuildTelegramPayload(ev Event) TelegramPayload` pure function, golden
tests under `internal/alert/testdata/`, and a `TelegramNotifier` that POSTs
to `https://api.telegram.org/bot<token>/sendMessage`.

**Acceptance criteria:**
- [ ] `config.TelegramAlert` with `BotTokenEnv` and `ChatID`, validated like
      the other channels (required fields when the section is present).
- [ ] `TelegramNotifier` implementing `alert.Notifier`.
- [ ] Golden payload tests for Down/Up/SSLExpiry events.
- [ ] `alert.NewManager` wires it up when configured.
- [ ] README alert-setup section gets a Telegram subsection.

---

## 2. SMS alerts via Twilio

**Labels:** `help wanted`

**Body:**

Add an SMS channel for on-call engineers who want a text message rather
than (or in addition to) Discord/Slack, using Twilio's REST API.

**Proposed shape:**

```yaml
alerts:
  sms:
    account_sid: "ACxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
    auth_token_env: "SITEWATCH_TWILIO_AUTH_TOKEN"
    from: "+15555550100"
    to:
      - "+15555550101"
```

Given SMS cost per message and the 160-character practical limit, only
Down and Up events should be eligible for SMS (SSL warnings stay on
Discord/Slack/email) — make that filterable per channel in a follow-up
rather than hardcoding it, but document the recommendation.

**Acceptance criteria:**
- [ ] `SMSNotifier` implementing `alert.Notifier`, POSTing to Twilio's
      `Messages` endpoint with HTTP Basic Auth (SID/token).
- [ ] Message body kept under ~150 characters (leave headroom for
      Twilio's own overhead).
- [ ] Golden test for the message body; delivery tested against an
      `httptest` server, not real Twilio.
- [ ] Config validation mirrors the email channel's "never store the
      secret in the file" pattern (`auth_token_env`, not `auth_token`).

---

## 3. DNS resolution check type

**Labels:** `help wanted`

**Body:**

Today sitewatch only does HTTP(S) checks. Add a `type: dns` check that
verifies a domain resolves (and optionally that it resolves to an expected
IP/CNAME), which catches a class of outage (DNS misconfiguration, expired
domain, broken registrar records) that an HTTP check on the same host can't
distinguish from "the server is down."

**Proposed shape:**

```yaml
checks:
  - name: "Acme Plumbing — DNS"
    type: dns
    host: "acmeplumbing.example.com"
    expected_ip: "203.0.113.10"   # optional; omit to just check "resolves at all"
    record_type: A                 # A, AAAA, or CNAME
    interval: 5m
    timeout: 5s
```

**Acceptance criteria:**
- [ ] `config.Check.Type` field (`"http"` default, `"dns"` new), validated
      so `dns` checks require `host` instead of `url`.
- [ ] A `dnschecker` package using `net.Resolver` with a context timeout
      (mirroring `internal/checker`'s do/Evaluate split so the pass/fail
      logic is unit-testable without a real DNS server).
- [ ] Tests use a fake `net.Resolver`-shaped interface, not real DNS
      lookups (a custom `LookupIPFunc`/`LookupCNAMEFunc` injected the same
      way `checker.Checker` injects its `*http.Client`).
- [ ] Wired into the same state machine / alert / history pipeline as HTTP
      checks (a DNS failure is still a Down event).

---

## 4. Regex body matching (beyond plain substring)

**Labels:** `good first issue`

**Body:**

`body_contains` today is a plain substring match. Some sites need something
slightly richer — e.g. "the price shown is a number" or "the page does NOT
contain 'Error 500'" — without reaching for a full scripting layer.

**Proposed shape:**

```yaml
checks:
  - name: "Acme Store — Pricing"
    url: "https://store.example.com"
    body_matches: 'Price: \$\d+\.\d{2}'   # regexp; mutually exclusive with body_contains
    body_not_contains: "Error 500"          # negative match
```

**Acceptance criteria:**
- [ ] `config.Check.BodyMatches` (compiled once at load time via
      `regexp.Compile`, with a validation error including the check's
      line number if the pattern doesn't compile) and `BodyNotContains`.
- [ ] `checker.Evaluate` gains the new checks, same pure-function pattern
      as the existing `BodyContains` check — unit tests with fabricated
      `Metrics`, no real HTTP.
- [ ] Config validation rejects setting both `body_contains` and
      `body_matches` on the same check (pick one).

---

## 5. Maintenance windows / alert silencing

**Labels:** `help wanted`

**Body:**

There's currently no way to silence alerts during planned maintenance
(a deploy, a host migration) without editing the config file and losing
history continuity. Add a way to mark a check (or all checks) as silenced
for a window, so `sitewatch run` keeps recording history and computing
uptime but suppresses alerts during that window — and probably excludes
that window from the uptime percentage shown on the status page, so a
planned deploy doesn't tank a client-visible uptime number.

**Proposed shape:**

```yaml
checks:
  - name: "Acme Plumbing — Homepage"
    url: "https://acmeplumbing.example.com"
    maintenance_windows:
      - start: "2026-04-01T02:00:00Z"
        end: "2026-04-01T03:00:00Z"
        reason: "Planned host migration"
```

**Acceptance criteria:**
- [ ] `config.Check.MaintenanceWindows []MaintenanceWindow{Start, End,
      Reason time.Time/string}`.
- [ ] `state.CheckState` (or the daemon's alert-dispatch call site) checks
      "is `now` inside a maintenance window for this check?" before
      calling `alerts.Send`, with a unit test covering the boundary
      (window start/end are inclusive/exclusive — pick one and test it).
- [ ] `history.PercentInWindow` gains an option to exclude maintenance
      windows from the uptime denominator, with a dedicated test.
- [ ] Documented in the README config reference.

---

## 6. Prometheus metrics endpoint

**Labels:** `help wanted`

**Body:**

For users who already run Prometheus/Grafana, expose sitewatch's own
current state as metrics rather than (or alongside) the static HTML status
page, so it can be scraped and alerted on through existing infra.

**Proposed shape:** `sitewatch run --metrics-addr :9469` starts a minimal
`net/http` server (std lib only — no `client_golang` dependency, to keep
sitewatch's zero-extra-dependency philosophy) exposing text-format metrics
compatible with the Prometheus exposition format:

```
# HELP sitewatch_up Whether the check is currently up (1) or down (0).
# TYPE sitewatch_up gauge
sitewatch_up{check="Acme Plumbing — Homepage"} 1
# HELP sitewatch_response_time_ms Most recent response time in milliseconds.
# TYPE sitewatch_response_time_ms gauge
sitewatch_response_time_ms{check="Acme Plumbing — Homepage"} 182
```

**Acceptance criteria:**
- [ ] A small hand-written exposition-format writer (no new dependency),
      with a golden test on the output format.
- [ ] `--metrics-addr` flag on `sitewatch run`, off by default.
- [ ] Metrics reflect the daemon's live in-memory state, not the history
      file (no need to re-parse JSONL on every scrape).
- [ ] README gets a short "Prometheus" section.

---

## 7. PagerDuty alert channel

**Labels:** `good first issue`, `help wanted`

**Body:**

Add PagerDuty as an alert channel via the Events API v2, for users who
already have on-call rotations set up there instead of (or in addition to)
Discord/Slack/email.

**Proposed shape:**

```yaml
alerts:
  pagerduty:
    routing_key_env: "SITEWATCH_PAGERDUTY_ROUTING_KEY"
```

Down events should trigger an incident (`event_action: trigger`); Up
events should resolve it (`event_action: resolve`), using the check name
as a stable `dedup_key` so PagerDuty correctly correlates the pair.
SSL-expiry warnings map to `event_action: trigger` at `severity: warning`
without an automatic resolve (they're informational, not a service
disruption).

**Acceptance criteria:**
- [ ] `PagerDutyNotifier` implementing `alert.Notifier`, POSTing to
      `https://events.pagerduty.com/v2/enqueue`.
- [ ] `dedup_key` is a stable, deterministic function of the check name
      (documented and tested) so trigger/resolve pairs correlate.
- [ ] Golden payload tests for trigger and resolve bodies.
- [ ] Config validation requires `routing_key_env` (never store the
      routing key itself in the file, consistent with every other secret
      in sitewatch).
