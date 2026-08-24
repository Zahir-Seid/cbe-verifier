package cbeverifier

import (
	"errors"
	"time"
)

// Backend identifies which CBE service produced a verification result.
type Backend string

const (
	// BackendLegacyPDF is the legacy receipt service at apps.cbe.com.et:100,
	// which returns a PDF document.
	BackendLegacyPDF Backend = "legacy_pdf"

	// BackendJSONAPI is the modern receipt service at mb.cbe.com.et, which
	// returns JSON.
	BackendJSONAPI Backend = "json_api"
)

// Status is the outcome of a verification attempt. It is the primary field
// callers should branch on.
type Status string

const (
	// StatusValid means the receipt was fetched and every compared field
	// matches the caller-supplied transaction.
	StatusValid Status = "valid"

	// StatusMismatch means the receipt was fetched successfully but at
	// least one compared field differs. Inspect Result.Mismatches.
	StatusMismatch Status = "mismatch"

	// StatusNotFound means CBE has no receipt for the supplied reference
	// or token (HTTP 404 from either backend).
	StatusNotFound Status = "not_found"
)

// Transaction is the caller-supplied data to verify against official CBE
// records.
type Transaction struct {
	// Reference is what the payer shared with you. It may be:
	//   - a bare legacy reference such as "FT25062PP5ZB" (Suffix required),
	//   - a full legacy receipt URL (suffix embedded, Suffix optional),
	//   - a new-platform token, or a full mbreciept.cbe.com.et URL.
	Reference string `json:"reference"`
	// Suffix is the trailing digits of the payer's CBE account (the part
	// after "1000"). Required for bare legacy references; ignored when the
	// reference already embeds it or when verifying a new token.
	Suffix string `json:"suffix,omitempty"`
	// Amount is the amount you expect the payment to be, in ETB. Must be
	// greater than zero; it is the value compared against the official
	// receipt (rounded to 2 decimal places on both sides).
	Amount float64 `json:"amount"`
}

// TransactionDetails holds the official transaction information as recorded
// by CBE, normalized across both backends.
type TransactionDetails struct {
	// Payer is the debit account holder's name (title-cased for legacy
	// PDF receipts, as printed by CBE for JSON receipts).
	Payer string `json:"payer"`
	// PayerAccount is the masked debit account number, e.g. "1000****6789".
	PayerAccount string `json:"payer_account"`
	// Receiver is the credit account holder's name.
	Receiver string `json:"receiver"`
	// ReceiverAccount is the masked credit account number.
	ReceiverAccount string `json:"receiver_account"`
	// Amount is the credited amount in ETB.
	Amount float64 `json:"amount"`
	// Date is the payment date parsed to UTC where the source format was
	// recognized; the zero time otherwise.
	Date time.Time `json:"date"`
	// DateRaw is the date exactly as it appeared in the source.
	DateRaw string `json:"date_raw,omitempty"`
	// Reference is the official CBE reference number ("FT..." for both
	// backends).
	Reference string `json:"reference"`
	// Reason is the payment reason / type of service, if present.
	Reason string `json:"reason,omitempty"`
}

// Mismatch describes one compared field whose official value differs from
// the value supplied in the Transaction.
type Mismatch struct {
	// Field is the logical field name: "reference" or "amount".
	Field string `json:"field"`
	// Provided is the caller-supplied value.
	Provided any `json:"provided"`
	// Official is the value recorded by CBE.
	Official any `json:"official"`
}

// Result is the outcome of a verification attempt.
//
// A non-nil error return from [Verify] signals caller misuse or an
// infrastructure failure (network, service outage, unparseable receipt) —
// see the package sentinel errors. Business outcomes, including a receipt
// that does not exist or one that disagrees with the supplied amount, are
// reported here as data.
type Result struct {
	// Status is the outcome: valid, mismatch, or not_found.
	Status Status `json:"status"`
	// Valid reports whether Status == StatusValid.
	Valid bool `json:"valid"`
	// Backend records which CBE service answered.
	Backend Backend `json:"backend"`
	// Details holds the official record whenever the receipt was fetched
	// and parsed successfully (i.e. for valid and mismatch outcomes).
	Details *TransactionDetails `json:"details,omitempty"`
	// Mismatches lists the differing fields for StatusMismatch; empty
	// otherwise.
	Mismatches []Mismatch `json:"mismatches,omitempty"`
}

// Sentinel errors returned (wrapped) by [Verify]. Match with errors.Is;
// never compare error strings.
var (
	ErrEmptyReference       = errors.New("cbe: reference must not be empty")
	ErrUnsupportedReference = errors.New("cbe: unrecognized reference format")
	ErrMissingSuffix        = errors.New("cbe: legacy reference requires account suffix")
	ErrInvalidAmount        = errors.New("cbe: amount must be greater than zero")
	ErrNetwork              = errors.New("cbe: network error contacting CBE")
	ErrUnexpectedResponse   = errors.New("cbe: unexpected response from CBE")
	ErrReceiptParse         = errors.New("cbe: could not parse receipt")
	ErrServiceUnavailable   = errors.New("cbe: CBE service unavailable after retries")
)

// errTokenNotFound is internal: an HTTP 404 from either backend is mapped
// by Verify into a Result with StatusNotFound rather than surfaced here.
var errTokenNotFound = errors.New("cbe: receipt not found")
