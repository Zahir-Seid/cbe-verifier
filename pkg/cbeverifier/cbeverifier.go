// Package cbeverifier verifies Commercial Bank of Ethiopia (CBE) payment
// receipts against the bank's official records.
//
// Two CBE backends are supported, selected automatically from the shape of
// the caller-supplied reference:
//
//   - the legacy PDF receipt service (apps.cbe.com.et:100), addressed by a
//     reference such as "FT25062PP5ZB" plus the payer's account suffix, or
//     by a full pasted receipt URL;
//   - the modern JSON API (mb.cbe.com.et), addressed by a receipt token or
//     a pasted mbreciept.cbe.com.et URL.
//
// Verify is the single entry point:
//
//	result, err := cbeverifier.Verify(ctx, cbeverifier.Transaction{
//	    Reference: "FT25062PP5ZB",
//	    Suffix:    "12345678",
//	    Amount:    1500.00,
//	})
//	if err != nil {
//	    // Caller misuse (bad reference, missing suffix) or an
//	    // infrastructure failure (network, CBE outage, unreadable
//	    // receipt). Match with errors.Is against this package's
//	    // sentinel errors.
//	    return err
//	}
//	switch result.Status {
//	case cbeverifier.StatusValid:
//	    // result.Details holds the official record.
//	case cbeverifier.StatusMismatch:
//	    // result.Mismatches lists which fields disagree.
//	case cbeverifier.StatusNotFound:
//	    // CBE has no receipt for that reference/token.
//	}
//
// # Comparison semantics
//
// The supplied Amount is always compared against the official amount
// (both rounded to two decimal places). For legacy receipts the official
// reference number is additionally compared against the FT-reference
// embedded in the input. New-platform tokens are opaque and are not the
// official reference, so only the amount is compared there.
//
// # Error model
//
// Verify returns a non-nil error only for caller misuse or infrastructure
// failure; business outcomes are data on Result. This keeps retry/metrics
// handling at call sites explicit while making "receipt disagrees with the
// claim" a normal value to branch on.
//
// # Security posture
//
// TLS certificate verification is always on; [WithInsecureSkipVerify]
// exists purely as an escape hatch and should never be enabled in
// production. Response bodies are size-capped and every request honours
// the caller's context.
package cbeverifier

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Verify fetches the official CBE record for tx.Reference and checks it
// against tx.Amount.
//
// See the package documentation for the error model and comparison
// semantics. Options are documented on their constructors in http.go;
// with no options the JSON backend retries transient failures 4 times
// 1.8s apart and each HTTP round trip is bounded by [DefaultTimeout].
func Verify(ctx context.Context, tx Transaction, opts ...Option) (*Result, error) {
	s := resolveSettings(opts)

	reference := strings.TrimSpace(tx.Reference)
	if reference == "" {
		return nil, fmt.Errorf("validate transaction: %w", ErrEmptyReference)
	}
	if tx.Amount <= 0 {
		return nil, fmt.Errorf("validate transaction: %w", ErrInvalidAmount)
	}

	classified := ClassifyReference(reference)

	switch classified.Kind {
	case RefNewToken:
		details, err := fetchTransactionJSON(ctx, s, classified.Token)
		if err != nil {
			if errors.Is(err, errTokenNotFound) {
				return &Result{Status: StatusNotFound, Backend: BackendJSONAPI}, nil
			}
			return nil, fmt.Errorf("fetch receipt: %w", err)
		}
		// Tokens are opaque and are not the official reference; only the
		// amount is compared.
		return compare("", tx.Amount, details, BackendJSONAPI), nil

	case RefLegacy, RefLegacyURL:
		suffix := strings.TrimSpace(tx.Suffix)
		if suffix == "" {
			suffix = classified.Suffix
		}
		if suffix == "" {
			return nil, fmt.Errorf("validate transaction: %w", ErrMissingSuffix)
		}

		pdfBytes, err := fetchLegacyReceipt(ctx, s, classified.ID, suffix)
		if err != nil {
			return nil, fmt.Errorf("fetch receipt: %w", err)
		}
		details, err := parseReceipt(pdfBytes)
		if err != nil {
			return nil, fmt.Errorf("parse receipt: %w", err)
		}
		// Compare against the extracted FT-reference so pasted URLs do
		// not mismatch against their own formatting.
		return compare(classified.ID, tx.Amount, details, BackendLegacyPDF), nil

	default:
		return nil, fmt.Errorf("validate transaction: %w (%q)", ErrUnsupportedReference, reference)
	}
}

// compare evaluates the fetched details against what the caller claims. An
// empty claimedReference skips the reference comparison (token flow).
func compare(claimedReference string, claimedAmount float64, details *TransactionDetails, backend Backend) *Result {
	var mismatches []Mismatch

	if claimedReference != "" && claimedReference != details.Reference {
		mismatches = append(mismatches, Mismatch{
			Field:    "reference",
			Provided: claimedReference,
			Official: details.Reference,
		})
	}
	if round2(claimedAmount) != round2(details.Amount) {
		mismatches = append(mismatches, Mismatch{
			Field:    "amount",
			Provided: round2(claimedAmount),
			Official: round2(details.Amount),
		})
	}

	status := StatusValid
	if len(mismatches) > 0 {
		status = StatusMismatch
	}
	return &Result{
		Status:     status,
		Valid:      status == StatusValid,
		Backend:    backend,
		Details:    details,
		Mismatches: mismatches,
	}
}
