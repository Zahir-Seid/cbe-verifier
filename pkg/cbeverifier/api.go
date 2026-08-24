package cbeverifier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// cbeTransactionResponse mirrors the JSON body returned by CBE's public
// transaction-detail endpoint. Fields are optional: the API omits them on
// records it cannot fully resolve.
type cbeTransactionResponse struct {
	ID                  string   `json:"id"`
	DebitAccountHolder  string   `json:"debitAccountHolder"`
	DebitAccountNo      string   `json:"debitAccountNo"`
	CreditAccountHolder string   `json:"creditAccountHolder"`
	CreditAccountNo     string   `json:"creditAccountNo"`
	AmountCredited      string   `json:"amountCredited"`
	DateTimes           []string `json:"dateTimes"`
	PaymentDetails      []string `json:"paymentDetails"`
}

// dateLayouts lists the timestamp shapes observed from the JSON backend,
// tried in order.
var dateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
}

// fetchTransactionJSON retrieves and normalizes one receipt from the modern
// JSON backend. Transient failures (transport errors or HTTP
// 429/502/503/504) are retried per the configured policy; a 404 maps to the
// internal errTokenNotFound sentinel, which Verify converts into a
// not-found Result.
func fetchTransactionJSON(ctx context.Context, s *settings, token string) (*TransactionDetails, error) {
	url := joinAPIURL(s.apiBaseURL) + url.PathEscape(token)
	client := s.client()

	var lastErr error
	for attempt := range s.retryAttempts {
		if attempt > 0 {
			if err := sleepContext(ctx, s.retryDelay); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrNetwork, err)
			}
		}

		details, retryable, err := fetchTransactionOnce(ctx, client, s, url)
		if err == nil {
			return details, nil
		}
		if !retryable {
			return nil, err
		}
		lastErr = err
	}

	return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, lastErr)
}

// fetchTransactionOnce performs a single request. The retryable return
// value distinguishes transient failures (worth another attempt) from
// permanent ones (returned immediately).
func fetchTransactionOnce(ctx context.Context, client *http.Client, s *settings, url string) (details *TransactionDetails, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", "https://mbreciept.cbe.com.et")
	req.Header.Set("Referer", "https://mbreciept.cbe.com.et/")
	req.Header.Set("x-app-id", s.appID)
	req.Header.Set("x-app-version", s.appVersion)

	resp, err := client.Do(req)
	if err != nil {
		// Transport-level failures (DNS, TLS, timeout, cancellation) are
		// considered transient, mirroring the reference implementation.
		return nil, true, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
		if err != nil {
			return nil, false, fmt.Errorf("%w: reading response: %v", ErrUnexpectedResponse, err)
		}
		if len(body) > maxResponseBodyBytes {
			return nil, false, fmt.Errorf("%w: response exceeds %d bytes", ErrUnexpectedResponse, maxResponseBodyBytes)
		}

		var payload cbeTransactionResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, false, fmt.Errorf("%w: decoding JSON: %v", ErrUnexpectedResponse, err)
		}
		return mapJSONReceipt(payload), false, nil

	case resp.StatusCode == http.StatusNotFound:
		return nil, false, errTokenNotFound

	case isRetryableStatus(resp.StatusCode):
		return nil, true, fmt.Errorf("%w: HTTP %d", ErrServiceUnavailable, resp.StatusCode)

	default:
		return nil, false, fmt.Errorf("%w: HTTP %d", ErrUnexpectedResponse, resp.StatusCode)
	}
}

// mapJSONReceipt normalizes the raw API payload. The official id becomes
// the canonical Reference; amountCredited arrives as a decimal string.
func mapJSONReceipt(p cbeTransactionResponse) *TransactionDetails {
	date := time.Time{}
	dateRaw := ""
	if len(p.DateTimes) > 0 {
		dateRaw = strings.TrimSpace(p.DateTimes[0])
		for _, layout := range dateLayouts {
			if parsed, err := time.Parse(layout, dateRaw); err == nil {
				date = parsed.UTC()
				break
			}
		}
	}

	return &TransactionDetails{
		Payer:           strings.TrimSpace(p.DebitAccountHolder),
		PayerAccount:    strings.TrimSpace(p.DebitAccountNo),
		Receiver:        strings.TrimSpace(p.CreditAccountHolder),
		ReceiverAccount: strings.TrimSpace(p.CreditAccountNo),
		Amount:          parseDecimal(p.AmountCredited),
		Date:            date,
		DateRaw:         dateRaw,
		Reference:       strings.TrimSpace(p.ID),
		Reason:          strings.TrimSpace(strings.Join(p.PaymentDetails, " ")),
	}
}

// parseDecimal parses an amount string such as "1234.50" or "1,234.50",
// returning 0 for anything unparseable.
func parseDecimal(value string) float64 {
	cleaned := strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	amount, err := strconv.ParseFloat(cleaned, 64)
	if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0
	}
	return amount
}

// round2 rounds to two decimal places, correctly across zero and negatives
// (the previous implementation truncated via int conversion).
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// joinAPIURL guarantees exactly one trailing slash before the token path
// segment, tolerating base URLs supplied with or without one.
func joinAPIURL(base string) string {
	return strings.TrimSuffix(base, "/") + "/"
}

func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
