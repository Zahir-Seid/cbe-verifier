package cbeverifier

import (
	"net/url"
	"regexp"
	"strings"
)

// RefKind classifies a caller-supplied reference string.
type RefKind int

const (
	// RefUnknown means the input matches no known CBE reference shape.
	RefUnknown RefKind = iota
	// RefLegacy is a bare legacy reference, e.g. "FT25062PP5ZB". The
	// account suffix must be supplied separately by the caller.
	RefLegacy
	// RefLegacyURL is a full legacy receipt URL whose "id" query parameter
	// embeds both the reference and the 8-digit suffix.
	RefLegacyURL
	// RefNewToken is a new-platform token or a full mbreciept.cbe.com.et
	// URL containing one.
	RefNewToken
)

// Reference is the classified form of the caller's raw input.
type Reference struct {
	Kind RefKind
	// Token is set for RefNewToken.
	Token string
	// ID is the legacy reference ("FT...", uppercased); set for both
	// legacy kinds.
	ID string
	// Suffix is the 8-digit account suffix embedded in a legacy URL;
	// empty for bare legacy references.
	Suffix string
}

// The patterns mirror the reference implementation's cbeReference.ts
// exactly; change them together with that file, never independently.
var (
	newCBEURLRegex          = regexp.MustCompile(`(?i)^https?://mbreciept\.cbe\.com\.et/([A-Za-z0-9-]+)/?$`)
	newCBETokenRegex        = regexp.MustCompile(`^[A-Za-z0-9-]{15,40}$`)
	legacyCBEReferenceRegex = regexp.MustCompile(`(?i)^FT[A-Z0-9]{10}$`)
	legacyCombinedIDRegex   = regexp.MustCompile(`(?i)^(FT[A-Z0-9]{10})(\d{8})$`)
)

// ClassifyReference inspects a raw reference (which may be a bare code or a
// pasted URL) and reports which CBE backend it belongs to and how to use it.
//
// Detection order matters and mirrors verifyCBE.ts: a full legacy receipt
// URL wins first, then new-platform tokens (plain tokens must not start
// with "FT", case-insensitively), then bare legacy references. Anything
// else is RefUnknown.
func ClassifyReference(input string) Reference {
	trimmed := strings.TrimSpace(input)

	if ref, ok := extractLegacyURLData(trimmed); ok {
		return Reference{Kind: RefLegacyURL, ID: ref.ID, Suffix: ref.Suffix}
	}

	if token, ok := extractNewCBEToken(trimmed); ok {
		return Reference{Kind: RefNewToken, Token: token}
	}

	if legacyCBEReferenceRegex.MatchString(trimmed) {
		return Reference{Kind: RefLegacy, ID: strings.ToUpper(trimmed)}
	}

	return Reference{Kind: RefUnknown}
}

// extractNewCBEToken returns the new-platform token embedded in a pasted
// mbreciept.cbe.com.et URL, or the trimmed input itself when it is shaped
// like a bare token (15-40 chars of [A-Za-z0-9-] not starting with "FT").
func extractNewCBEToken(input string) (string, bool) {
	if m := newCBEURLRegex.FindStringSubmatch(input); m != nil {
		return m[1], true
	}

	if !strings.HasPrefix(strings.ToUpper(input), "FT") && newCBETokenRegex.MatchString(input) {
		return input, true
	}

	return "", false
}

// extractLegacyURLData parses a pasted legacy receipt URL of the form
// https://apps.cbe.com.et:100/?id=<FT...><8 digits> and splits it into the
// reference and account suffix. Any other URL shape is rejected.
func extractLegacyURLData(input string) (Reference, bool) {
	parsed, err := url.Parse(input)
	if err != nil {
		return Reference{}, false
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Reference{}, false
	}

	if parsed.Hostname() != "apps.cbe.com.et" {
		return Reference{}, false
	}

	if port := parsed.Port(); port != "" && port != "100" {
		return Reference{}, false
	}

	combined := strings.TrimSpace(parsed.Query().Get("id"))

	m := legacyCombinedIDRegex.FindStringSubmatch(combined)
	if m == nil {
		return Reference{}, false
	}

	return Reference{ID: strings.ToUpper(m[1]), Suffix: m[2]}, true
}
