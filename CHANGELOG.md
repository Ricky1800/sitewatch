# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Regex response-body matching with `body_matches` and negative substring
  checks with `body_not_contains`.

## [0.2.0] - 2026-09-27

### Added

- `status_page` config block (`title`, `logo_url`, `accent_color`,
  `description`) for cosmetic control over the generated status page,
  with sensible defaults so it's entirely optional.
- Overall-status banner ("All Systems Operational" / "Partial Outage" /
  "Major Outage" / "No Checks Configured") derived from every check's
  latest state.
- Per-check 90-day uptime bar: one segment per calendar day, colored by
  that day's uptime, with a hover/keyboard-focus tooltip showing the
  exact date and percentage (also exposed via `aria-label` so status is
  never conveyed by color alone).
- Incident history per check, derived from down↔up transitions in the
  history file (no separate incident log): start/end time, duration, and
  the last error seen, including a still-open "ongoing" incident.
- "Last updated" timestamp for the page itself (separate from each
  check's own "last checked").

### Changed

- Status page redesign: replaced the single table with a status-banner +
  per-service card layout, styled to the standard of a modern hosted
  status page. Still zero external CSS/JS/fonts, still light/dark aware
  via `prefers-color-scheme`, still responsive down to phone width.
- `internal/status.BuildPageData` / `PageData` / `CheckStatus` gained new
  fields (`OverallState`, `UptimeBar`, `Incidents`, `Title`, `LogoURL`,
  `AccentColor`, `Description`) — additive; existing fields are unchanged.

## [0.1.0] - 2026-09-26

### Added

- Initial release: single-binary uptime, SSL-expiry, and content monitor.
- `sitewatch.yaml` config with global defaults, per-check overrides, and
  helpful validation errors annotated with the offending YAML line number.
- HTTP checks: expected status code, `body_contains` substring match,
  `max_response_ms` response-time budget, TLS certificate expiry.
- Alert channels: Discord webhook, Slack webhook, generic JSON webhook, SMTP
  email (password read from an environment variable named in config, never
  stored in the file).
- Up/down state machine: alerts only on state *change* (down after N
  consecutive failures, recovery with downtime duration), so a flapping
  check doesn't spam alerts. SSL-expiry warnings fire at most once per day
  per check.
- `sitewatch run`: the monitoring daemon, one jittered goroutine scheduler
  per check, graceful shutdown on SIGINT/SIGTERM.
- `sitewatch check`: one-shot run for CI/cron, table or `--json` output,
  exit code 1 on any failure.
- `sitewatch status --out status.html`: static, self-contained HTML status
  page (embedded template, inline SVG response-time sparklines, 24h/7d/30d
  uptime).
- `sitewatch version`.
- Append-only JSONL history file with automatic daily compaction.
- Dockerfile (multi-stage, distroless/static), example systemd unit,
  GitHub Actions CI (gofmt/vet/test/race/build) and a tag-triggered release
  workflow.

[0.2.0]: https://github.com/Ricky1800/sitewatch/releases/tag/v0.2.0
[0.1.0]: https://github.com/Ricky1800/sitewatch/releases/tag/v0.1.0
