package cbeverifier

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVerify_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tx   Transaction
		want error
	}{
		{"empty reference", Transaction{Reference: "  ", Amount: 10}, ErrEmptyReference},
		{"zero amount", Transaction{Reference: "FT25062PP5ZB", Suffix: "12345678"}, ErrInvalidAmount},
		{"negative amount", Transaction{Reference: "FT25062PP5ZB", Amount: -5}, ErrInvalidAmount},
		{"unrecognized format", Transaction{Reference: "not a ref", Amount: 10}, ErrUnsupportedReference},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := Verify(context.Background(), tt.tx)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want wrapped %v", err, tt.want)
			}
			if result != nil {
				t.Errorf("result = %+v, want nil on caller misuse", result)
			}
		})
	}
}

func TestVerify_LegacyWithoutSuffixIsACallerError(t *testing.T) {
	t.Parallel()

	_, err := Verify(context.Background(), Transaction{Reference: "FT25062PP5ZB", Amount: 10})
	if !errors.Is(err, ErrMissingSuffix) {
		t.Fatalf("err = %v, want wrapped ErrMissingSuffix", err)
	}
}

func legacyTestServer(t *testing.T, lines []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(buildReceiptPDF(lines))
	}))
}

func TestVerify_LegacyMatchingReceiptIsValid(t *testing.T) {
	t.Parallel()

	server := legacyTestServer(t, fullReceiptLines())
	defer server.Close()

	result, err := Verify(context.Background(), Transaction{
		Reference: "FT25062PP5ZB",
		Suffix:    "12345678",
		Amount:    1234.50,
	}, withBaseURL(server.URL, ""))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.Status != StatusValid || !result.Valid {
		t.Fatalf("Status = %s (%+v), want valid", result.Status, result.Mismatches)
	}
	if result.Backend != BackendLegacyPDF {
		t.Errorf("Backend = %s, want legacy_pdf", result.Backend)
	}
	if result.Details == nil || result.Details.Reference != "FT25062PP5ZB" {
		t.Errorf("Details = %+v, want populated official record", result.Details)
	}
	if len(result.Mismatches) != 0 {
		t.Errorf("Mismatches = %+v, want empty", result.Mismatches)
	}
}

func TestVerify_LegacyAmountMismatch(t *testing.T) {
	t.Parallel()

	server := legacyTestServer(t, fullReceiptLines())
	defer server.Close()

	result, err := Verify(context.Background(), Transaction{
		Reference: "FT25062PP5ZB",
		Suffix:    "12345678",
		Amount:    999.99,
	}, withBaseURL(server.URL, ""))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.Status != StatusMismatch || result.Valid {
		t.Fatalf("Status = %s, want mismatch", result.Status)
	}
	if len(result.Mismatches) != 1 || result.Mismatches[0].Field != "amount" {
		t.Fatalf("Mismatches = %+v, want a single amount mismatch", result.Mismatches)
	}
	if result.Details == nil {
		t.Error("Details = nil, want the official record even on mismatch")
	}
}

func TestVerify_LegacyReferenceMismatch(t *testing.T) {
	t.Parallel()

	server := legacyTestServer(t, fullReceiptLines())
	defer server.Close()

	result, err := Verify(context.Background(), Transaction{
		Reference: "FT99999AAAAA", // receipt carries FT25062PP5ZB
		Suffix:    "12345678",
		Amount:    1234.50,
	}, withBaseURL(server.URL, ""))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.Status != StatusMismatch {
		t.Fatalf("Status = %s, want mismatch", result.Status)
	}
	fields := map[string]bool{}
	for _, m := range result.Mismatches {
		fields[m.Field] = true
	}
	if !fields["reference"] {
		t.Errorf("Mismatches = %+v, want a reference entry", result.Mismatches)
	}
}

func TestVerify_LegacyURLSuppliesSuffix(t *testing.T) {
	t.Parallel()

	server := legacyTestServer(t, fullReceiptLines())
	defer server.Close()

	var gotID string
	wrapped := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = r.URL.Query().Get("id")
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(buildReceiptPDF(fullReceiptLines()))
	}))
	defer wrapped.Close()
	_ = gotID

	result, err := Verify(context.Background(), Transaction{
		// Suffix deliberately empty: it is embedded in the URL below.
		Reference: "https://apps.cbe.com.et:100/?id=FT25062PP5ZB12345678",
		Amount:    1234.50,
	}, withBaseURL(wrapped.URL, ""))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if result.Status != StatusValid {
		t.Fatalf("Status = %s (%+v), want valid", result.Status, result.Mismatches)
	}
}

func TestVerify_JSONAPITokenFlow(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "FT25063QQ9XC",
			"debitAccountHolder": "Bekele Abebe",
			"debitAccountNo": "1000****1111",
			"creditAccountHolder": "Chaltu Gemechu",
			"creditAccountNo": "1001****2222",
			"amountCredited": "500.00",
			"dateTimes": ["2025-06-25T09:00:00Z"],
			"paymentDetails": ["Groceries"]
		}`))
	}))
	defer server.Close()

	result, err := Verify(context.Background(), Transaction{
		Reference: "AbCdEf123456789",
		Amount:    500.00,
	}, withBaseURL("", server.URL))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.Status != StatusValid {
		t.Fatalf("Status = %s (%+v), want valid", result.Status, result.Mismatches)
	}
	if result.Backend != BackendJSONAPI {
		t.Errorf("Backend = %s, want json_api", result.Backend)
	}
	// The token is opaque and must NOT be compared against the official id.
	if result.Details.Reference != "FT25063QQ9XC" {
		t.Errorf("Details.Reference = %q, want the official FT reference", result.Details.Reference)
	}
}

func TestVerify_JSONAPINotFoundIsData(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	result, err := Verify(context.Background(), Transaction{
		Reference: "missing-token-0000",
		Amount:    100,
	}, withBaseURL("", server.URL), WithRetry(4, time.Millisecond))
	if err != nil {
		t.Fatalf("err = %v, want nil for a business outcome", err)
	}
	if result.Status != StatusNotFound || result.Valid {
		t.Fatalf("Status = %s Valid = %v, want not_found/false", result.Status, result.Valid)
	}
}

func TestVerify_CancelledContextIsInfrastructureError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	server := legacyTestServer(t, fullReceiptLines())
	defer server.Close()

	_, err := Verify(ctx, Transaction{
		Reference: "FT25062PP5ZB",
		Suffix:    "12345678",
		Amount:    1234.50,
	}, withBaseURL(server.URL, ""))
	if !errors.Is(err, ErrNetwork) && !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want wrapped network/context error", err)
	}
}

func TestRound2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   float64
		want float64
	}{
		{1234.504, 1234.50},
		{1234.505, 1234.51},
		{10.005, 10.01}, // would truncate wrongly under int-cast rounding
		{-3.005, -3.01},
		{-3.004, -3.00},
		{0.001, 0},
	}
	for _, tt := range tests {
		if got := round2(tt.in); got != tt.want {
			t.Errorf("round2(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
