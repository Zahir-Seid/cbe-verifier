package cbeverifier

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFetchTransactionJSON_MapsReceipt(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "FT25063QQ9XC",
			"debitAccountHolder": "  Abebe Kebede ",
			"debitAccountNo": "1000****6789",
			"creditAccountHolder": "Almaz Tesfaye",
			"creditAccountNo": "1001****4321",
			"amountCredited": "1,250.75",
			"dateTimes": ["2025-06-24T15:41:07Z"],
			"paymentDetails": ["House Rent", "June 2025"]
		}`))
	}))
	defer server.Close()

	details, err := fetchTransactionJSON(context.Background(),
		resolveSettings([]Option{withBaseURL("", server.URL)}), testNewToken)
	if err != nil {
		t.Fatalf("fetchTransactionJSON: %v", err)
	}

	assertDetailMatches(t, details, detailChecks{
		payer:           "Abebe Kebede",
		payerAccount:    "1000****6789",
		receiver:        "Almaz Tesfaye",
		receiverAccount: "1001****4321",
		amount:          1250.75,
		reference:       "FT25063QQ9XC",
		reason:          "House Rent June 2025",
	})
	wantDate := time.Date(2025, 6, 24, 15, 41, 7, 0, time.UTC)
	if !details.Date.Equal(wantDate) {
		t.Errorf("Date = %v, want %v", details.Date, wantDate)
	}
}

func TestFetchTransactionJSON_RetriesTransientStatuses(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"id":"FT1","amountCredited":"10.00"}`))
	}))
	defer server.Close()

	opts := []Option{withBaseURL("", server.URL), WithRetry(4, time.Millisecond)}
	if _, err := fetchTransactionJSON(context.Background(), resolveSettings(opts), "token-token-tok"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3 (two retries then success)", attempts)
	}
}

func TestFetchTransactionJSON_RetryExhaustionWrapsSentinel(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			t.Parallel()

			var attempts int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts++
				w.WriteHeader(status)
			}))
			defer server.Close()

			opts := []Option{withBaseURL("", server.URL), WithRetry(3, time.Millisecond)}
			_, err := fetchTransactionJSON(context.Background(), resolveSettings(opts), "token-token-tok")
			if !errors.Is(err, ErrServiceUnavailable) {
				t.Fatalf("err = %v, want wrapped ErrServiceUnavailable", err)
			}
			if attempts != 3 {
				t.Errorf("attempts = %d, want 3", attempts)
			}
		})
	}
}

func TestFetchTransactionJSON_NotFoundIsImmediate(t *testing.T) {
	t.Parallel()

	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	opts := []Option{withBaseURL("", server.URL), WithRetry(4, time.Millisecond)}
	_, err := fetchTransactionJSON(context.Background(), resolveSettings(opts), "missing-token-00")
	if !errors.Is(err, errTokenNotFound) {
		t.Fatalf("err = %v, want errTokenNotFound", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (404 must not retry)", attempts)
	}
}

func TestFetchTransactionJSON_NonRetryableStatusFailsFast(t *testing.T) {
	t.Parallel()

	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	opts := []Option{withBaseURL("", server.URL), WithRetry(4, time.Millisecond)}
	_, err := fetchTransactionJSON(context.Background(), resolveSettings(opts), "token-token-tok")
	if !errors.Is(err, ErrUnexpectedResponse) {
		t.Fatalf("err = %v, want wrapped ErrUnexpectedResponse", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}

func TestFetchTransactionJSON_MalformedJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	defer server.Close()

	opts := []Option{withBaseURL("", server.URL)}
	_, err := fetchTransactionJSON(context.Background(), resolveSettings(opts), "token-token-tok")
	if !errors.Is(err, ErrUnexpectedResponse) {
		t.Fatalf("err = %v, want wrapped ErrUnexpectedResponse", err)
	}
}

func TestFetchTransactionJSON_SendsExpectedHeaders(t *testing.T) {
	t.Parallel()

	var got http.Header
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		close(done)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	opts := []Option{withBaseURL("", server.URL), WithAppCredentials("my-app-id", "my-app-version")}
	_, err := fetchTransactionJSON(context.Background(), resolveSettings(opts), "token-token-tok")
	if err != nil {
		t.Fatalf("fetchTransactionJSON: %v", err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("server never received a request")
	}

	expectations := map[string]string{
		"User-Agent":    userAgent,
		"Accept":        "application/json, text/plain, */*",
		"Origin":        "https://mbreciept.cbe.com.et",
		"Referer":       "https://mbreciept.cbe.com.et/",
		"X-App-Id":      "my-app-id",
		"X-App-Version": "my-app-version",
	}
	for key, want := range expectations {
		if gotValue := got.Get(key); gotValue != want {
			t.Errorf("header %s = %q, want %q", key, gotValue, want)
		}
	}
}

func TestFetchLegacyReceipt_RejectsNonPDFResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>error page</html>"))
	}))
	defer server.Close()

	opts := []Option{withBaseURL(server.URL, "")}
	_, err := fetchLegacyReceipt(context.Background(), resolveSettings(opts), "FT25062PP5ZB", "12345678")
	if !errors.Is(err, ErrUnexpectedResponse) {
		t.Fatalf("err = %v, want wrapped ErrUnexpectedResponse", err)
	}
}

func TestFetchLegacyReceipt_BuildsIDQuery(t *testing.T) {
	t.Parallel()

	var gotRawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/pdf")
		receipt := buildReceiptPDF([]string{
			"Payer : X",
			"Account : 1000****0000",
			"Receiver : Y",
			"Account : 1001****1111",
			"Transferred Amount : 5.00 ETB",
			"Reference No. (VAT Invoice No) : FT25062PP5ZB",
			"Payment Date & Time : 6/24/2025, 3:41:07 PM",
		})
		_, _ = w.Write(receipt)
	}))
	defer server.Close()

	opts := []Option{withBaseURL(server.URL, "")}
	body, err := fetchLegacyReceipt(context.Background(), resolveSettings(opts), "FT25062PP5ZB", "12345678")
	if err != nil {
		t.Fatalf("fetchLegacyReceipt: %v", err)
	}
	if !strings.HasPrefix(gotRawQuery, "id=FT25062PP5ZB12345678") {
		t.Errorf("RawQuery = %q, want id=FT25062PP5ZB12345678...", gotRawQuery)
	}
	if len(body) == 0 {
		t.Error("body empty, want PDF bytes")
	}
	if json.Valid(body) {
		t.Error("body is JSON, want a PDF document")
	}
}
