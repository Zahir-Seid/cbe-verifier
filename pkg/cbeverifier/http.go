package cbeverifier

import (
	"crypto/tls"
	"net/http"
	"time"
)

// Defaults mirroring verifyCBE.ts. The app id/version identify CBE's own
// public receipt web app; they are constants there and overridable here for
// controlled rotation.
const (
	// DefaultTimeout bounds a single HTTP round trip to either backend.
	DefaultTimeout = 30 * time.Second

	defaultRetryAttempts = 4
	defaultRetryDelay    = 1800 * time.Millisecond

	defaultAppID      = "d1292e42-7400-49de-a2d3-9731caa4c819"
	defaultAppVersion = "0a01980b-9859-1369-8198-59f403820000"

	legacyReceiptBaseURL  = "https://apps.cbe.com.et:100/"
	apiTransactionBaseURL = "https://mb.cbe.com.et/api/v1/transactions/public/transaction-detail/"

	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"

	// maxResponseBodyBytes caps how much we read from any single response,
	// bounding memory against a misbehaving server. Real receipts are a
	// few tens of kilobytes.
	maxResponseBodyBytes = 10 << 20 // 10 MiB
)

// settings carries the resolved configuration for one Verify call.
type settings struct {
	timeout            time.Duration
	insecureSkipVerify bool
	retryAttempts      int
	retryDelay         time.Duration
	appID              string
	appVersion         string
	legacyBaseURL      string
	apiBaseURL         string
}

// withBaseURL overrides one or both backend endpoints. It is unexported on
// purpose: production callers always hit CBE, while white-box tests point it
// at httptest servers.
func withBaseURL(legacy, api string) Option {
	return func(s *settings) {
		if legacy != "" {
			s.legacyBaseURL = legacy
		}
		if api != "" {
			s.apiBaseURL = api
		}
	}
}

func resolveSettings(opts []Option) *settings {
	s := &settings{
		timeout:       DefaultTimeout,
		retryAttempts: defaultRetryAttempts,
		retryDelay:    defaultRetryDelay,
		appID:         defaultAppID,
		appVersion:    defaultAppVersion,
		legacyBaseURL: legacyReceiptBaseURL,
		apiBaseURL:    apiTransactionBaseURL,
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.timeout <= 0 {
		s.timeout = DefaultTimeout
	}
	if s.retryAttempts < 1 {
		s.retryAttempts = 1
	}
	if s.retryDelay < 0 {
		s.retryDelay = 0
	}
	return s
}

// Option configures a single call to [Verify].
type Option func(*settings)

// WithTimeout bounds each HTTP round trip; values <= 0 fall back to
// [DefaultTimeout].
func WithTimeout(d time.Duration) Option {
	return func(s *settings) { s.timeout = d }
}

// WithInsecureSkipVerify disables TLS certificate verification for both CBE
// endpoints.
//
// Security: this makes every request vulnerable to man-in-the-middle attacks
// and exists only as an escape hatch for environments whose TLS inspection
// proxies break certificate chains. Do not enable it in production without
// understanding exactly why you need it.
func WithInsecureSkipVerify() Option {
	return func(s *settings) { s.insecureSkipVerify = true }
}

// WithRetry sets how many times the JSON backend is retried on transient
// failures (transport errors or HTTP 429/502/503/504) and how long to wait
// between attempts. attempts < 1 becomes 1 (no retries).
func WithRetry(attempts int, delay time.Duration) Option {
	return func(s *settings) {
		s.retryAttempts = attempts
		s.retryDelay = delay
	}
}

// WithAppCredentials overrides the x-app-id / x-app-version headers sent to
// the JSON backend. Defaults mirror CBE's public receipt app.
func WithAppCredentials(appID, appVersion string) Option {
	return func(s *settings) {
		s.appID = appID
		s.appVersion = appVersion
	}
}

// httpClient builds the HTTP client used for a call, applying the timeout
// and TLS posture from the resolved settings.
func (s *settings) client() *http.Client {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12} //nolint:gosec // minimum TLS 1.2 is intentional; InsecureSkipVerify stays false unless explicitly opted into
	if s.insecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true //nolint:gosec // explicit caller opt-in via WithInsecureSkipVerify
	}
	return &http.Client{
		Timeout: s.timeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
}
