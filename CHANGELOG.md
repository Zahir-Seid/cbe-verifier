# Changelog

All notable changes to this project are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [2.0.0] - Unreleased

Complete redesign. The module path gains the `/v2` suffix; import as
`github.com/Zahir-Seid/cbe-verifier/v2/pkg/cbeverifier`.

### Added

- Support for CBE's modern JSON receipt API (`mb.cbe.com.et`): tokens and
  pasted `mbreciept.cbe.com.et` URLs are verified against
  `/api/v1/transactions/public/transaction-detail/{token}`, with the
  reference implementation's retry policy carried over (4 attempts, 1.8 s
  apart, retrying transport errors and HTTP 429/502/503/504; 404 is an
  immediate not-found).
- Automatic backend routing from the shape of the supplied reference:
  bare legacy references, full legacy receipt URLs, tokens, and pasted
  modern-platform URLs are all accepted by the same `Verify` call.
- `context.Context` support: every request honours caller cancellation.
- Functional options (`WithTimeout`, `WithRetry`, `WithAppCredentials`,
  `WithInsecureSkipVerify`) replacing the fixed `Options` struct.
- Typed `Result`/`TransactionDetails`/`Mismatch` contract shared by both
  backends; official records are normalized (amount parsed from string,
  dates parsed to UTC where recognized, names title-cased).
- Table-driven test suite with `httptest` servers covering parsing,
  routing, retries, header contracts, comparison semantics, and end-to-end
  flows for both backends.
- CI: multi-version Go matrix with race/shuffle detection, golangci-lint
  (v2 config, 48 linters), govulncheck, gosec, and Dependabot.

### Changed

- `Verify` now takes a `context.Context` first parameter.
- Comparison semantics: amount always compared (2-decimal rounding,
  correct across negatives); legacy receipts additionally compare the
  FT-reference extracted from the input; opaque new-platform tokens are
  never compared to the official reference.
- TLS certificate verification is strict by default;
  `InsecureSkipVerify` was unconditional in v1 and is now an explicit,
  documented opt-in.

### Fixed

- Sentinel errors are real wrapped errors reachable via `errors.Is`; in
  v1 they were exported but never returned (all failures were flattened
  into `result.Error` strings).
- Amount rounding no longer truncates through integer conversion, which
  mis-rounded values such as `10.005` and all negative amounts.
- Receipt parsing no longer writes temporary files and bounds response
  reads at 10 MiB; URL query parameters are properly escaped.

### Removed

- `Options` struct, `DefaultOptions()`, `ParseCBEReceipt`,
  `VerifyResult`, and the `map[string]interface{}` result shape.

## [1.0.0]

Historical release: legacy-PDF-only verification. Superseded by 2.0.0;
the v1 module remains available under its original import path.
