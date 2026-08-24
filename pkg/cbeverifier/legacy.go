package cbeverifier

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// fetchLegacyReceipt downloads the raw PDF receipt for reference+suffix
// from the legacy apps.cbe.com.et:100 service. It returns the PDF bytes;
// parsing happens in parser.go.
func fetchLegacyReceipt(ctx context.Context, s *settings, reference, suffix string) ([]byte, error) {
	endpoint := s.legacyBaseURL + "?" + url.Values{"id": {reference + suffix}}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/pdf")
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	defer func() { _ = resp.Body.Close() }()

	contentType := resp.Header.Get("Content-Type")
	if resp.StatusCode != http.StatusOK || !containsPDFMediaType(contentType) {
		return nil, fmt.Errorf("%w: HTTP %d with content type %q", ErrUnexpectedResponse, resp.StatusCode, contentType)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: reading receipt: %v", ErrNetwork, err)
	}
	if len(body) > maxResponseBodyBytes {
		return nil, fmt.Errorf("%w: receipt exceeds %d bytes", ErrUnexpectedResponse, maxResponseBodyBytes)
	}
	return body, nil
}

// containsPDFMediaType reports whether a Content-Type header designates a
// PDF document (CBE has been observed to send parameters such as charset).
func containsPDFMediaType(contentType string) bool {
	return mediaTypeIs(contentType, "application/pdf")
}
