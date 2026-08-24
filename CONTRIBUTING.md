# Contributing to cbe-verifier

The maintainer's handbook: philosophy, architecture, and the playbooks for
keeping this library correct against a moving target — two CBE backends
that can change without notice.

## 1. Scope & philosophy

One job: given what a payer claims (reference + amount), fetch the official
record from CBE and report whether they agree. Everything else — payment
flows, storage of results, user-facing copy — belongs to callers.

Non-negotiables:

1. **Errors are real errors.** Caller misuse and infrastructure failures
   return wrapped sentinel errors (`errors.Is`-able). Business outcomes —
   mismatch, not-found — are data on `Result`. Never flatten one into the
   other.
2. **Context first.** Every network path propagates the caller's
   `context.Context`; cancellation must always work.
3. **Strict TLS by default.** `WithInsecureSkipVerify` exists for broken
   TLS-inspection environments only; treat any widening of it as a security
   regression.
4. **Bounded everything.** Response bodies are size-capped, retries are
   finite, timeouts have defaults.
5. **Both backends stay first-class.** Legacy receipts still circulate;
   routing, parsing, and tests must cover legacy PDF and JSON API equally.
6. **No logging in the library.** Payment metadata is sensitive; callers
   decide what to record.

## 2. Architecture tour

| File | Role |
| --- | --- |
| `cbeverifier.go` | `Verify` orchestrator: validate → classify → fetch → parse → compare. Owns the error model. |
| `reference.go` | `ClassifyReference`: ports `utils/cbeReference.ts` regex-for-regex. Detection order is load-bearing (legacy URL → token → bare legacy → unknown); change it together with the TS file, never alone. |
| `api.go` | Modern JSON client: retry loop (transport errors + 429/502/503/504; 404 short-circuits), required headers (`x-app-id`, `x-app-version`, Origin/Referer), payload mapping. |
| `legacy.go` / `contenttype.go` | Legacy PDF download with content-type validation and size caps. |
| `parser.go` | Label-driven extraction over normalized receipt text. Patterns are tuned against real receipts; see the masking-shape note before "simplifying" them. |
| `http.go` | Settings resolution + functional options + shared HTTP client (TLS posture, timeouts). |
| `types.go` | The published contract. JSON tags are API surface. |

Tests mirror source files one-to-one (`x.go` ↔ `x_test.go`), use table
drives with named subtests, `httptest` servers injected via the unexported
`withBaseURL` option, and synthetic receipt PDFs generated programmatically
in `parser_test.go` — no binary fixtures in git.

**Never let a test hit the network.** A mis-shaped fixture string that
accidentally classifies as a real reference will fire live requests at CBE.
If an e2e test fails with a several-second delay, suspect exactly that.

## 3. Maintenance playbooks

### When the JSON API changes

Field drift surfaces as zero-valued `TransactionDetails` fields. Update the
`cbeTransactionResponse` struct, `mapJSONReceipt`, and add the captured
response shape (sanitized) as a test fixture.

### When app credentials rotate

Defaults live in `http.go` (`defaultAppID`, `defaultAppVersion`) mirroring
CBE's public receipt app. Rotate them there; callers can override per-call
via `WithAppCredentials` meanwhile.

### When the legacy receipt layout changes

Symptom: `ErrReceiptParse` with missing-field lists on receipts that used
to verify. Capture the new layout, extend the label patterns in
`parser.go`, and add both old and new shapes as generated fixtures. Beware:
extraction glues adjacent rows — patterns must tolerate labels running into
values, which is why account masks capture whole digit/star runs.

### If `dslipak/pdf` breaks

It is unmaintained upstream; if it stops parsing receipts, candidate
replacements are `ledongthuc/pdf` or vendoring a fork. Swap behind
`parseReceipt`'s signature so the rest of the package doesn't move.

### Release checklist

1. `go vet ./... && go test -race -shuffle=on ./...` green; lint locally
   with golangci-lint v2 if installed (CI enforces otherwise).
2. Behavior changes have tests; README updated.
3. Bump versions only via tags on `main`; update CHANGELOG first.
4. Major-version discipline: this module is `/v2`; a future breaking
   change means `/v3` — module line, imports, README, go get docs.

## 4. Branching & release flow

Identical to the organization standard used across these repositories:

- `main` — production; protected; releases tagged here only.
- `develop` — integration branch; PRs required.
- `feature/<short-kebab>`, `fix/<short-kebab>` — branched off `develop`,
  merged back via PR, deleted after merge.
- `hotfix/<short-kebab>` — branched off `main`; PR to `main`, then
  backported to `develop`.

Release ritual: `develop` green → PR `develop → main` → tag `vX.Y.Z` on
`main` → GitHub Release with CHANGELOG notes → ping the proxy
(`GOPROXY=https://proxy.golang.org go list -m github.com/Zahir-Seid/cbe-verifier/v2@vX.Y.Z`)
so pkg.go.dev indexes immediately.

Golden rules: never commit directly to `main`, never work on `develop`
directly, keep branches small, production stays stable.

## 5. Legal note

This library interacts with CBE's public verification endpoints and
processes payment metadata supplied by callers. Integrators are responsible
for lawful purpose limitation, retention, and consent under Ethiopia's
Personal Data Protection Proclamation (No. 1321/2024) when storing
verification results. Not affiliated with or endorsed by the Commercial
Bank of Ethiopia.
