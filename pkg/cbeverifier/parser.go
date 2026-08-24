package cbeverifier

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	pdf "github.com/dslipak/pdf"
)

// The patterns mirror parseCBEReceipt in verifyCBE.ts, tuned against real
// CBE receipts: account numbers are printed masked, names may be glued to
// following labels by text extraction, and labels carry optional colons.
var (
	rePayerName   = regexp.MustCompile(`(?i)Payer\s*:?\s*(.*?)\s+Account`)
	reReceiverNam = regexp.MustCompile(`(?i)Receiver\s*:?\s*(.*?)\s+Account`)
	// Masked accounts appear in several shapes across receipt revisions
	// (e.g. "1000****6789", "***4567", "**********1234"). Capture the
	// whole digit/star run rather than assuming a fixed star count - the
	// reference implementation's stricter pattern misses some of these.
	reAccounts = regexp.MustCompile(`(?i)Account\s*:?\s*([\d*]+)`)
	reReason   = regexp.MustCompile(`(?i)Reason\s*/\s*Type of service\s*:?\s*(.*?)\s+Transferred Amount`)
	reAmount   = regexp.MustCompile(`(?i)Transferred Amount\s*:?\s*([\d,]+\.\d{2})\s*ETB`)
	// The reference value itself stays case-sensitive: under (?i) the
	// class would swallow a following glued label ("...PP5ZBPayment").
	reReference   = regexp.MustCompile(`(?i)Reference No\.?\s*\(VAT Invoice No\)\s*:?\s*((?-i)[A-Z0-9]+)`)
	rePaymentDate = regexp.MustCompile(`(?i)Payment Date & Time\s*:?\s*([\d/:, ]+[AP]M)`)

	reCamelBoundary = regexp.MustCompile(`([a-z])([A-Z])`)

	legacyDateLayouts = []string{
		"1/2/2006, 3:04:05 PM",
		"01/02/2006, 03:04:05 PM",
		"2/1/2006, 15:04:05",
	}
)

// parseReceipt extracts the official transaction details from a legacy PDF
// receipt's bytes.
//
// The whole document's text is normalized into a single line (the way the
// reference implementation feeds pdf-parse) and fields are located with the
// label-driven patterns above. A receipt missing any required field yields
// an error wrapping [ErrReceiptParse]; the reason field alone is optional.
func parseReceipt(pdfBytes []byte) (*TransactionDetails, error) {
	if !bytes.HasPrefix(bytes.TrimSpace(pdfBytes), []byte("%PDF-")) {
		return nil, fmt.Errorf("%w: not a PDF document", ErrReceiptParse)
	}

	doc, err := pdf.NewReader(bytes.NewReader(pdfBytes), int64(len(pdfBytes)))
	if err != nil {
		return nil, fmt.Errorf("%w: opening document: %w", ErrReceiptParse, err)
	}

	text := normalizeDocumentText(extractAllText(doc))

	details := &TransactionDetails{
		Payer:           titleCase(firstMatch(text, rePayerName)),
		PayerAccount:    nthMatch(text, reAccounts, 0),
		Receiver:        titleCase(firstMatch(text, reReceiverNam)),
		ReceiverAccount: nthMatch(text, reAccounts, 1),
		Amount:          parseDecimal(firstMatch(text, reAmount)),
		DateRaw:         firstMatch(text, rePaymentDate),
		Reference:       firstMatch(text, reReference),
		Reason:          firstMatch(text, reReason),
	}
	details.Date = parseLegacyDate(details.DateRaw)

	var missing []string
	for field, value := range map[string]string{
		"payer":            details.Payer,
		"payer_account":    details.PayerAccount,
		"receiver":         details.Receiver,
		"receiver_account": details.ReceiverAccount,
		"reference":        details.Reference,
	} {
		if value == "" {
			missing = append(missing, field)
		}
	}
	if details.Amount <= 0 {
		missing = append(missing, "amount")
	}
	if details.Date.IsZero() {
		missing = append(missing, "date")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: missing %s", ErrReceiptParse, strings.Join(missing, ", "))
	}
	return details, nil
}

// extractAllText concatenates every page's text rows, repairing camelCase
// words that text extraction glues together ("PayerName" -> "Payer Name").
func extractAllText(doc *pdf.Reader) string {
	var rows []string
	for i := 1; i <= doc.NumPage(); i++ {
		page := doc.Page(i)
		if page.V.IsNull() {
			continue
		}
		pageRows, err := page.GetTextByRow()
		if err != nil {
			continue
		}
		for _, row := range pageRows {
			rows = append(rows, repairGluedWords(joinWords(row.Content)))
		}
	}
	return strings.Join(rows, " ")
}

// normalizeDocumentText collapses all whitespace runs to single spaces so
// label patterns can span line breaks, exactly like the reference
// implementation's `replace(/\s+/g, ' ')`.
func normalizeDocumentText(raw string) string {
	return strings.Join(strings.Fields(raw), " ")
}

func joinWords(words []pdf.Text) string {
	var sb strings.Builder
	for i, word := range words {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(word.S)
	}
	return sb.String()
}

func repairGluedWords(line string) string {
	return reCamelBoundary.ReplaceAllString(line, "$1 $2")
}

func firstMatch(text string, re *regexp.Regexp) string {
	return nthMatch(text, re, 0)
}

func nthMatch(text string, re *regexp.Regexp, n int) string {
	matches := re.FindAllStringSubmatch(text, -1)
	if len(matches) <= n || len(matches[n]) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[n][1])
}

// titleCase normalizes extracted names to Title Case, matching the
// reference implementation.
func titleCase(name string) string {
	if name == "" {
		return ""
	}
	words := strings.Fields(strings.ToLower(name))
	for i, word := range words {
		runes := []rune(word)
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}

// parseLegacyDate parses the receipt's printed date, e.g.
// "6/24/2025, 3:41:07 PM", returning the zero time when unrecognized.
func parseLegacyDate(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	for _, layout := range legacyDateLayouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}
