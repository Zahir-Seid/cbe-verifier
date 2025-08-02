// Package cbeverifier provides functionality to verify Commercial Bank of Ethiopia (CBE)
// transaction receipts by fetching and parsing official PDF documents.
package cbeverifier

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	pdf "github.com/dslipak/pdf"
)

// VerifyResult represents the result of parsing a CBE receipt PDF
type VerifyResult struct {
	// Success indicates whether the PDF was successfully parsed
	Success bool `json:"success"`
	// Details contains the extracted transaction information or error details
	Details map[string]interface{} `json:"details"`
}

// Precompiled regex patterns for extracting transaction information
var (
	// rePayer matches payer information in the receipt
	rePayer = regexp.MustCompile(`(?i)payer\s*[:]?\s*([\w\s&\.-]+)`)

	// reReceiver matches receiver information in the receipt
	reReceiver = regexp.MustCompile(`(?i)receiver\s*[:]?\s*([\w\s&\.-]+)`)

	// reAccount matches account numbers in the receipt
	reAccount = regexp.MustCompile(`(?i)account\s*[:]?\s*(\S+)`)

	// reTransferredAmt matches transferred amount in ETB
	reTransferredAmt = regexp.MustCompile(`(?i)transferred amount\s*[:]?\s*([\d,]+\.\d{2})\s*ETB`)

	// reReason matches payment reason/description
	reReason = regexp.MustCompile(`(?i)reason\s*[:]?\s*(.+)`)

	// reReferenceNo matches reference number
	reReferenceNo = regexp.MustCompile(`(?i)reference no\.?\s*[:]?\s*(.+)`)

	// rePaymentDate matches payment date and time
	rePaymentDate = regexp.MustCompile(`(?i)payment date.*?(\d{1,2}/\d{1,2}/\d{4}(?:,\s*\d{1,2}:\d{2}:\d{2}\s*(?:AM|PM)?)?)`)

	// reParenthetical removes parenthetical content
	reParenthetical = regexp.MustCompile(`^\(.*?\)`)

	// reFixMergedWords fixes merged words by inserting spaces
	reFixMergedWords = regexp.MustCompile(`([a-z])([A-Z])`)
)

// ParseCBEReceipt parses a CBE receipt PDF and extracts transaction information
//
// This function:
// 1. Creates a temporary file from the provided PDF bytes
// 2. Opens and processes the PDF using the pdf library
// 3. Extracts transaction details using regex patterns
// 4. Returns structured transaction information
//
// The function returns a VerifyResult with:
// - Success: true if parsing was successful
// - Details: map containing extracted fields or error information
//
// Example:
//
//	pdfBytes, err := os.ReadFile("receipt.pdf")
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	result := cbeverifier.ParseCBEReceipt(pdfBytes)
//	if result.Success {
//		fmt.Printf("Amount: %.2f ETB\n", result.Details["amount"])
//		fmt.Printf("Payer: %s\n", result.Details["payer"])
//	} else {
//		fmt.Printf("Parse error: %v\n", result.Details["error"])
//	}
func ParseCBEReceipt(pdfBytes []byte) VerifyResult {
	// Validate PDF header
	if !strings.HasPrefix(string(pdfBytes), "%PDF-") {
		return VerifyResult{
			Success: false,
			Details: map[string]interface{}{
				"error": "invalid PDF format: missing PDF header",
			},
		}
	}

	// Create temporary file for PDF processing
	tmpfile, err := os.CreateTemp("", "cbe-*.pdf")
	if err != nil {
		return VerifyResult{
			Success: false,
			Details: map[string]interface{}{
				"error": fmt.Sprintf("could not create temp file: %v", err),
			},
		}
	}
	defer os.Remove(tmpfile.Name())

	// Write PDF content to temporary file
	if _, err := tmpfile.Write(pdfBytes); err != nil {
		return VerifyResult{
			Success: false,
			Details: map[string]interface{}{
				"error": fmt.Sprintf("could not write to temp file: %v", err),
			},
		}
	}
	tmpfile.Close()

	// Open PDF document
	doc, err := pdf.Open(tmpfile.Name())
	if err != nil {
		return VerifyResult{
			Success: false,
			Details: map[string]interface{}{
				"error": fmt.Sprintf("failed to open PDF: %v", err),
			},
		}
	}

	// Extract transaction information
	details := extractTransactionDetails(doc)

	// Validate extracted information
	if isValidTransaction(details) {
		return VerifyResult{
			Success: true,
			Details: details,
		}
	}

	// Return error with missing field information
	return VerifyResult{
		Success: false,
		Details: map[string]interface{}{
			"error":   "missing one or more required fields",
			"missing": getMissingFields(details),
		},
	}
}

// extractTransactionDetails processes the PDF document and extracts transaction information
func extractTransactionDetails(doc *pdf.Reader) map[string]interface{} {
	var (
		payer, receiver, transferredAmt, reason, refNo, paymentDate string
		payerAccounts, receiverAccounts                             []string
		currentEntity                                               string
	)

	// Process each page of the PDF
	for i := 1; i <= doc.NumPage(); i++ {
		page := doc.Page(i)
		if page.V.IsNull() {
			continue
		}

		// Get text content by rows
		rows, err := page.GetTextByRow()
		if err != nil {
			continue
		}

		// Process each row of text
		for _, row := range rows {
			line := joinWords(row.Content)
			line = fixLineSpacing(line)

			// Extract different fields based on regex patterns
			switch {
			case extractField(line, rePayer) != "":
				payer = extractField(line, rePayer)
				currentEntity = "payer"

			case extractField(line, reReceiver) != "":
				receiver = extractField(line, reReceiver)
				currentEntity = "receiver"

			case extractField(line, reAccount) != "":
				account := extractField(line, reAccount)
				if currentEntity == "payer" {
					payerAccounts = append(payerAccounts, account)
				} else if currentEntity == "receiver" {
					receiverAccounts = append(receiverAccounts, account)
				}

			case extractField(line, reTransferredAmt) != "":
				transferredAmt = extractField(line, reTransferredAmt)

			case extractField(line, reReason) != "":
				reason = extractReason(line)

			case extractField(line, reReferenceNo) != "":
				refNo = extractReferenceNumber(line)

			case extractField(line, rePaymentDate) != "":
				paymentDate = extractField(line, rePaymentDate)
			}
		}
	}

	// Parse amount from string to float64
	amount := parseAmount(transferredAmt)

	// Build result map
	return map[string]interface{}{
		"payer":           payer,
		"payerAccount":    getFirstAccount(payerAccounts),
		"receiver":        receiver,
		"receiverAccount": getFirstAccount(receiverAccounts),
		"amount":          amount,
		"date":            paymentDate,
		"transaction_id":  refNo,
		"reason":          reason,
	}
}

// Helper functions

// fixLineSpacing inserts spaces between merged words
func fixLineSpacing(line string) string {
	return reFixMergedWords.ReplaceAllString(line, "$1 $2")
}

// joinWords concatenates text fragments into a single string
func joinWords(words []pdf.Text) string {
	var sb strings.Builder
	for _, word := range words {
		sb.WriteString(word.S)
	}
	return sb.String()
}

// extractField applies a regex pattern to extract a field value
func extractField(line string, re *regexp.Regexp) string {
	if matches := re.FindStringSubmatch(line); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// extractReason extracts and cleans the payment reason
func extractReason(line string) string {
	rawReason := extractField(line, reReason)

	// Handle "Type of service" prefix
	if idx := strings.Index(rawReason, "Type of service"); idx != -1 {
		rawReason = rawReason[idx+len("Type of service"):]
		rawReason = strings.TrimLeft(rawReason, "/: \t")
	} else {
		// Find the last separator and extract content after it
		separators := []string{"/", ":"}
		lastPos := -1
		for _, sep := range separators {
			pos := strings.LastIndex(rawReason, sep)
			if pos > lastPos {
				lastPos = pos
			}
		}
		if lastPos >= 0 && lastPos+1 < len(rawReason) {
			rawReason = strings.TrimSpace(rawReason[lastPos+1:])
		}
	}

	return strings.TrimSpace(rawReason)
}

// extractReferenceNumber extracts and cleans the reference number
func extractReferenceNumber(line string) string {
	ref := extractField(line, reReferenceNo)
	ref = strings.TrimSpace(reParenthetical.ReplaceAllString(ref, ""))
	return ref
}

// parseAmount converts amount string to float64
func parseAmount(amountStr string) float64 {
	if amountStr == "" {
		return 0
	}

	// Remove commas and parse
	cleanAmount := strings.ReplaceAll(amountStr, ",", "")
	var amount float64
	fmt.Sscanf(cleanAmount, "%f", &amount)
	return amount
}

// getFirstAccount returns the first account from a slice, or empty string if none
func getFirstAccount(accounts []string) string {
	if len(accounts) > 0 {
		return accounts[0]
	}
	return ""
}

// isValidTransaction checks if all required fields are present
func isValidTransaction(details map[string]interface{}) bool {
	required := []string{"payer", "receiver", "payerAccount", "receiverAccount", "transaction_id", "date"}

	for _, field := range required {
		if value, exists := details[field]; !exists || value == "" {
			return false
		}
	}

	// Check if amount is greater than 0
	if amount, ok := details["amount"].(float64); !ok || amount <= 0 {
		return false
	}

	return true
}

// getMissingFields returns a map indicating which required fields are missing
func getMissingFields(details map[string]interface{}) map[string]interface{} {
	missing := make(map[string]interface{})

	// Check string fields
	stringFields := []string{"payer", "receiver", "payerAccount", "receiverAccount", "transaction_id", "date"}
	for _, field := range stringFields {
		if value, exists := details[field]; !exists || value == "" {
			missing[field] = true
		} else {
			missing[field] = false
		}
	}

	// Check amount
	if amount, ok := details["amount"].(float64); !ok || amount <= 0 {
		missing["amount"] = true
	} else {
		missing["amount"] = false
	}

	return missing
}
