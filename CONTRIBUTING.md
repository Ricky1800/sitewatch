# Contributing to sitewatch

Thanks for considering a contribution. sitewatch is intentionally small in
scope (a single static binary, std lib + one YAML dependency), so the bar
for new dependencies and new subcommands is high, but bug fixes, new alert
channels, new check types, and documentation improvements are all welcome.

## Getting started

```sh
git clone https://github.com/Ricky1800/sitewatch.git
cd sitewatch
go build ./...
go test ./...
```

Requires Go 1.22+.

## Before opening a PR

Run the same checks CI runs:

```sh
gofmt -l .            # must print nothing
go vet ./...
go test ./...
go test -race ./...   # requires cgo; may not work on Windows without a C toolchain
```

Please also:

- Add tests for new behavior. Existing tests avoid real network calls and
  real sleeps wherever the logic under test can be made deterministic (see
  `internal/clock` for the fake-clock pattern, and `internal/checker`'s
  split between `do` (network) and `Evaluate` (pure) for the pattern to
  follow when adding a new check type).
- Keep new alert channels behind the existing `alert.Notifier` interface,
  and add a golden payload test (see `internal/alert/alert_test.go`) rather
  than asserting on live network calls.
- Update `CHANGELOG.md` under an `Unreleased` heading.
- Keep the dependency list to the standard library plus `gopkg.in/yaml.v3`
  unless there's a very strong reason otherwise — that's discussed in an
  issue first, not just a PR.

## Commit style

This repo uses [Conventional Commits](https://www.conventionalcommits.org/)
(`feat:`, `fix:`, `docs:`, `chore:`, `test:`, ...). Keep commits focused —
one logical change per commit.

## Reporting bugs / requesting features

Use the issue templates. Check `ROADMAP_ISSUES.md` first — if what you want
is already scoped there, comment on it (or open a PR!) rather than filing a
duplicate.

## Code of conduct

Be respectful, assume good faith, and keep discussion focused on the
technical problem. Maintainers may edit or close issues/PRs that don't meet
that bar.
