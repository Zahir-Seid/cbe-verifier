package cbeverifier

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// buildReceiptPDF renders receipt lines into a minimal but well-formed
// single-page PDF (correct xref offsets, Helvetica text), suitable for
// github.com/dslipak/pdf text extraction.
func buildReceiptPDF(lines []string) []byte {
	var content strings.Builder
	content.WriteString("BT /F1 11 Tf 14 TL 40 760 Td\n")
	for _, line := range lines {
		content.WriteString("(" + escapePDFString(line) + ") Tj T*\n")
	}
	content.WriteString("ET")
	stream := content.String()

	var buf bytes.Buffer
	offsets := make([]int, 6)
	writeObject := func(num int, body string) {
		offsets[num] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", num, body)
	}

	buf.WriteString("%PDF-1.4\n")
	writeObject(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObject(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	writeObject(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "+
		"/Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>")
	writeObject(4, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	writeObject(5, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))

	xrefPos := buf.Len()
	buf.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefPos)
	return buf.Bytes()
}

func escapePDFString(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)
	return replacer.Replace(s)
}

// fullReceiptLines returns a synthetic-but-realistic CBE receipt body whose
// every field parses cleanly.
func fullReceiptLines() []string {
	return []string{
		"Commercial Bank of Ethiopia",
		"Payment Receipt",
		"Payer : Abebe Kebede Bekele",
		"Account : 1000****6789",
		"Receiver : Almaz Tesfaye Girma",
		"Account : 1001****4321",
		"Reason / Type of service : House Rent Payment",
		"Transferred Amount : 1,234.50 ETB",
		"Reference No. (VAT Invoice No) : FT25062PP5ZB",
		"Payment Date & Time : 6/24/2025, 3:41:07 PM",
	}
}

func TestParseReceipt_ValidFullReceipt(t *testing.T) {
	t.Parallel()

	details, err := parseReceipt(buildReceiptPDF(fullReceiptLines()))
	if err != nil {
		t.Fatalf("parseReceipt: %v", err)
	}

	assertDetailMatches(t, details, detailChecks{
		payer:           "Abebe Kebede Bekele",
		payerAccount:    "1000****6789",
		receiver:        "Almaz Tesfaye Girma",
		receiverAccount: "1001****4321",
		amount:          1234.50,
		reference:       testLegacyReference,
		reason:          "House Rent Payment",
		dateRaw:         "6/24/2025, 3:41:07 PM",
	})
	if details.Date.IsZero() {
		t.Errorf("Date = zero, want parsed payment date")
	}
}

func TestParseReceipt_ReasonIsOptional(t *testing.T) {
	t.Parallel()

	lines := fullReceiptLines()
	filtered := lines[:6]
	filtered = append(filtered, lines[7:]...) // drop the Reason line only

	details, err := parseReceipt(buildReceiptPDF(filtered))
	if err != nil {
		t.Fatalf("parseReceipt without reason: %v", err)
	}
	if details.Reason != "" {
		t.Errorf("Reason = %q, want empty", details.Reason)
	}
}

func TestParseReceipt_MissingFieldsListedInError(t *testing.T) {
	t.Parallel()

	// Receipt without the receiver block and without the payment date.
	lines := []string{
		"Commercial Bank of Ethiopia",
		"Payment Receipt",
		"Payer : Abebe Kebede Bekele",
		"Account : 1000****6789",
		"Reason / Type of service : House Rent Payment",
		"Transferred Amount : 1,234.50 ETB",
		"Reference No. (VAT Invoice No) : FT25062PP5ZB",
	}

	_, err := parseReceipt(buildReceiptPDF(lines))
	if !errors.Is(err, ErrReceiptParse) {
		t.Fatalf("err = %v, want wrapped ErrReceiptParse", err)
	}
	for _, field := range []string{"receiver", "receiver_account", "date"} {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("error %q does not mention missing field %q", err, field)
		}
	}
}

func TestParseReceipt_RejectsNonPDF(t *testing.T) {
	t.Parallel()

	_, err := parseReceipt([]byte("<html>not a pdf</html>"))
	if !errors.Is(err, ErrReceiptParse) {
		t.Fatalf("err = %v, want wrapped ErrReceiptParse", err)
	}
	if !strings.Contains(err.Error(), "not a PDF") {
		t.Errorf("error %q should explain the document is not a PDF", err)
	}
}

// detailChecks enumerates the TransactionDetails fields a test cares about;
// zero-valued entries are skipped.
type detailChecks struct {
	payer           string
	payerAccount    string
	receiver        string
	receiverAccount string
	amount          float64
	dateRaw         string
	reference       string
	reason          string
}

func assertDetailMatches(t *testing.T, d *TransactionDetails, want detailChecks) {
	t.Helper()
	check := func(field, got, expected string) {
		t.Helper()
		if expected != "" && got != expected {
			t.Errorf("%s = %q, want %q", field, got, expected)
		}
	}
	check("Payer", d.Payer, want.payer)
	check("PayerAccount", d.PayerAccount, want.payerAccount)
	check("Receiver", d.Receiver, want.receiver)
	check("ReceiverAccount", d.ReceiverAccount, want.receiverAccount)
	check("DateRaw", d.DateRaw, want.dateRaw)
	check("Reference", d.Reference, want.reference)
	check("Reason", d.Reason, want.reason)
	if want.amount != 0 && d.Amount != want.amount {
		t.Errorf("Amount = %.2f, want %.2f", d.Amount, want.amount)
	}
}
