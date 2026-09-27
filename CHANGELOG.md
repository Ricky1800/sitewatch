# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

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

[0.1.0]: https://github.com/Ricky1800/sitewatch/releases/tag/v0.1.0
