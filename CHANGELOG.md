# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [2.1.0] - 2026-10-01

First tagged release, with prebuilt binaries for Linux, macOS and Windows.

### Added
- **Walk-forward optimisation** (`--optimize`): entry/exit thresholds tuned on a
  rolling 120-day window and scored only on the following 30 days, compared
  with fixed thresholds and buy-and-hold over the same out-of-sample period.
- **Volatility-targeted strategy**: same signals, entries sized to aim for 2%
  daily volatility (ATR-based); reported next to the others. (#12)
- **Average exposure** for every strategy, so half-invested and all-in
  strategies compare fairly. (#12)
- `--grid` for custom threshold grids, with validation. (#11)
- `--export` JSON and a **live dashboard** on GitHub Pages, rebuilt daily by
  GitHub Actions from CoinGecko data.
- Tests for the CoinGecko client against an `httptest` server. (#10)
- Release workflow: pushing a `v*` tag builds binaries with GoReleaser.

### Changed
- The CoinGecko client only retries network errors, 429 and 5xx; a typo in
  `--coins` (HTTP 404) now fails at once with a clear message, and
  `Retry-After` is honoured. (#10)
- Go module renamed to `github.com/wyplerszymon0-lab/cryptopulse`.
- `--version` reports the release tag in released binaries.

### Fixed
- **Sharpe and Sortino** were annualised with √252 (equity trading days);
  crypto trades every day, so √365.
- **Sortino** divided the downside deviation by losing days only instead of
  all days.
- The walk-forward result lost its exposure series, producing `NaN`, which
  would have broken the dashboard's JSON export. (#12)
- Coin IDs are path-escaped in API URLs. (#10)
- Misaligned terminal banner (padding counted bytes instead of characters).

## [2.0.0] - 2026-05-25

Backtesting engine with lookahead-free signals, 7 indicators in a weighted
composite score, concurrent fetching, Docker image and CI.

[2.1.0]: https://github.com/wyplerszymon0-lab/cryptopulse/releases/tag/v2.1.0
[2.0.0]: https://github.com/wyplerszymon0-lab/cryptopulse/commit/6d9f478
