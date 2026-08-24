package cbeverifier

import (
	"mime"
	"strings"
)

// mediaTypeIs reports whether the Content-Type header's media type equals
// want, ignoring parameters and case. Empty headers never match.
func mediaTypeIs(contentType, want string) bool {
	if strings.TrimSpace(contentType) == "" {
		return false
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		// Fall back to a tolerant comparison for non-conforming headers.
		return strings.EqualFold(strings.TrimSpace(strings.Split(contentType, ";")[0]), want)
	}

	return strings.EqualFold(mediaType, want)
}
