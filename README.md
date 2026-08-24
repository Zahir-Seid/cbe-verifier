# CBE Verifier Go Library

A Go library for verifying Commercial Bank of Ethiopia (CBE) payment
receipts against the bank's official records. It supports **both** CBE
platforms and picks the right one automatically from the shape of the
reference you give it:

| Input | Backend |
| --- | --- |
| Bare legacy reference (`FT25062PP5ZB`) + account suffix | Legacy PDF service (`apps.cbe.com.et:100`) |
| Full legacy receipt URL | Legacy PDF service (suffix extracted from the URL) |
| New-platform token or `mbreciept.cbe.com.et` URL | Modern JSON API (`mb.cbe.com.et`) |

## Install

```bash
go get github.com/Zahir-Seid/cbe-verifier/v2
```

> **Coming from v1?** The module path now carries the `/v2` suffix and the
> API has been redesigned — see [Migrating from v1](#migrating-from-v1).

## Quick start

```go
package main

import (
	"context"
	"fmt"

	cbeverifier "github.com/Zahir-Seid/cbe-verifier/v2/pkg/cbeverifier"
)

func main() {
	result, err := cbeverifier.Verify(context.Background(), cbeverifier.Transaction{
		Reference: "FT25062PP5ZB",
		Suffix:    "12345678", // digits after 1000 in the payer's CBE account
		Amount:    1500.00,
	})
	if err != nil {
		// Caller misuse or infrastructure failure - match with errors.Is.
		panic(err)
	}

	switch result.Status {
	case cbeverifier.StatusValid:
		fmt.Printf("verified via %s: paid by %s\n", result.Backend, result.Details.Payer)
	case cbeverifier.StatusMismatch:
		for _, m := range result.Mismatches {
			fmt.Printf("%s differs: claimed %v, official %v\n", m.Field, m.Provided, m.Official)
		}
	case cbeverifier.StatusNotFound:
		fmt.Println("CBE has no receipt for that reference")
	}
}
```

Verifying a new-platform receipt is the same call — paste whatever the
payer shared:

```go
result, err := cbeverifier.Verify(ctx, cbeverifier.Transaction{
	Reference: "https://mbreciept.cbe.com.et/AbCdEf123456789", // token auto-extracted
	Amount:    250.00,
})
```

## API

### `Verify(ctx context.Context, tx Transaction, opts ...Option) (*Result, error)`

The single entry point.

**Error model.** `err != nil` means caller misuse (empty/unknown reference,
missing suffix, non-positive amount) or an infrastructure failure (network,
CBE outage after retries, unreadable receipt). Match against the sentinel
errors with `errors.Is`:

`ErrEmptyReference`, `ErrUnsupportedReference`, `ErrMissingSuffix`,
`ErrInvalidAmount`, `ErrNetwork`, `ErrUnexpectedResponse`,
`ErrReceiptParse`, `ErrServiceUnavailable`.

Business outcomes are data on `Result`, never errors:

| `Result.Status` | Meaning |
| --- | --- |
| `StatusValid` | Receipt fetched; every compared field matches. |
| `StatusMismatch` | Receipt fetched; at least one field differs (`Result.Mismatches`). |
| `StatusNotFound` | CBE returned 404 for that reference/token. |

**Comparison semantics.** The supplied `Amount` is always compared against
the official amount (both rounded to two decimals). For legacy receipts the
official FT-reference is additionally compared against the one embedded in
your input. New-platform tokens are opaque and are *not* the official
reference, so only the amount is compared there.

### `Transaction`

```go
type Transaction struct {
	Reference string  // raw input: bare ref, pasted URL, or token
	Suffix    string  // required for bare legacy references only
	Amount    float64 // expected amount in ETB (> 0)
}
```

### `Result`

```go
type Result struct {
	Status     Status              // valid | mismatch | not_found
	Valid      bool                // convenience: Status == StatusValid
	Backend    Backend             // legacy_pdf | json_api
	Details    *TransactionDetails // official record (valid & mismatch)
	Mismatches []Mismatch          // [{field, provided, official}]
}
```

`TransactionDetails` normalizes both backends: payer/receiver names,
masked account numbers, amount, date (parsed to UTC where recognized, plus
`DateRaw` as printed), official reference, and reason.

### Options

| Option | Default | Purpose |
| --- | --- | --- |
| `WithTimeout(d)` | `30s` | Per-request HTTP timeout |
| `WithRetry(attempts, delay)` | `4`, `1.8s` | JSON-backend retry policy for transport errors and HTTP 429/502/503/504 (404 never retries) |
| `WithAppCredentials(id, version)` | CBE's public app ids | Overrides the `x-app-id` / `x-app-version` headers |
| `WithInsecureSkipVerify()` | off | **Dangerous**: disables TLS verification. Escape hatch only — do not enable in production |

## CLI

```bash
go run ./example --reference FT25062PP5ZB --suffix 12345678 --amount 1500
go run ./example --reference "https://mbreciept.cbe.com.et/AbCdEf123456789" --amount 250
```

Prints the JSON `Result`; exits `0` when valid, `1` when not verified, `2`
on usage/infrastructure errors.

## Requirements

- Go 1.24+
- Network access to CBE endpoints (verification is inherently online)

## Security notes

- TLS certificate verification is **always on** unless you explicitly pass
  `WithInsecureSkipVerify()` — which exists only for broken corporate TLS
  inspection setups and should never ship.
- Response bodies are size-capped (10 MiB) and every request honours your
  `context.Context`.
- Nothing is logged by the library; what you log is what leaks.

## Dependencies

- [`github.com/dslipak/pdf`](https://github.com/dslipak/pdf) — legacy
  receipt text extraction (see CONTRIBUTING for the maintenance caveat)

## Migrating from v1

| v1 | v2 |
| --- | --- |
| `module github.com/Zahir-Seid/cbe-verifier` | `github.com/Zahir-Seid/cbe-verifier/v2` |
| `cbeverifier.Verify(tx, opts)` | `cbeverifier.Verify(ctx, tx, opts...)` |
| `Options{IncludeDetails, Timeout}` struct | Functional options (`WithTimeout`, …); details are now always populated |
| Errors flattened into `result.Error` strings | Real wrapped errors; `errors.Is` works |
| Parser returns `map[string]interface{}` | Single typed `TransactionDetails` everywhere |
| `Mismatches map[string]interface{}` | Typed `[]Mismatch{Field, Provided, Official}` |
| New JSON API unsupported | Supported, with retries and token/URL routing |
| TLS verification disabled unconditionally | Strict by default, opt-out explicit |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) — branch model, release process,
and maintenance playbooks.

## License

MIT — see [LICENSE](LICENSE).

## Disclaimer

This library is an independent community tool and is not affiliated with
or endorsed by the Commercial Bank of Ethiopia. Verify high-value payments
through official channels; use at your own risk and ensure compliance with
CBE's terms of service.
