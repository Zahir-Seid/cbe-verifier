// Package cbeverifier provides functionality to verify Commercial Bank of Ethiopia (CBE)
// transaction receipts by fetching and parsing official PDF documents.
//
// This package allows you to:
//   - Fetch official CBE transaction receipts from the bank's servers
//   - Parse PDF documents to extract transaction details
//   - Compare provided transaction data with official records
//   - Verify transaction authenticity and accuracy
//
// Example usage:
//
//	package main
//
//	import (
//		"fmt"
//		"log"
//		"github.com/Zahir-Seid/cbe-verifier/cbeverifier"
//	)
//
//	func main() {
//		// Create transaction data to verify
//		transaction := cbeverifier.Transaction{
//			ID:     "xxxxxxxx",
//			Suffix: "xxxxxxxx",
//			Amount: xxxx.xx,
//		}
//
//		// Verify against official records
//		result, err := cbeverifier.Verify(transaction, cbeverifier.Options{
//			IncludeDetails: true,
//			Timeout:        120,
//		})
//
//		if err != nil {
//			log.Printf("Verification failed: %v", err)
//			return
//		}
//
//		if result.IsValid {
//			fmt.Printf("Transaction verified successfully!\n")
//			if result.Details != nil {
//				fmt.Printf("Official amount: %.2f ETB\n", result.Details.Amount)
//				fmt.Printf("Payer: %s\n", result.Details.Payer)
//				fmt.Printf("Receiver: %s\n", result.Details.Receiver)
//			}
//		} else {
//			fmt.Printf("Transaction verification failed: %s\n", result.Error)
//		}
//	}
package cbeverifier

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Common errors that may be returned by the library
var (
	ErrInvalidTransactionID = errors.New("invalid transaction ID")
	ErrInvalidSuffix        = errors.New("invalid suffix")
	ErrInvalidAmount        = errors.New("invalid amount")
	ErrNetworkError         = errors.New("network error while requesting CBE receipt")
	ErrInvalidPDFResponse   = errors.New("invalid PDF response from CBE")
	ErrPDFReadError         = errors.New("could not read PDF content")
	ErrReceiptParseError    = errors.New("failed to parse receipt")
	ErrVerificationFailed   = errors.New("transaction verification failed")
)

// Transaction represents a CBE transaction to be verified
type Transaction struct {
	// ID is the transaction reference number (e.g., "xxxxxxxx")
	ID string `json:"id"`
	// Suffix is the transaction suffix (e.g., "xxxxxxxx")
	Suffix string `json:"suffix"`
	// Amount is the transaction amount in ETB
	Amount float64 `json:"amount"`
}

// Options configures the verification process
type Options struct {
	// IncludeDetails returns the full transaction details from the official receipt
	IncludeDetails bool `json:"include_details"`
	// Timeout specifies the HTTP request timeout in seconds (default: 120)
	Timeout int `json:"timeout"`
}

// DefaultOptions returns the default verification options
func DefaultOptions() Options {
	return Options{
		IncludeDetails: false,
		Timeout:        120,
	}
}

// TransactionDetails contains the parsed transaction information from the official receipt
type TransactionDetails struct {
	// Payer is the name of the person/entity making the payment
	Payer string `json:"payer"`
	// PayerAccount is the account number of the payer
	PayerAccount string `json:"payer_account"`
	// Receiver is the name of the person/entity receiving the payment
	Receiver string `json:"receiver"`
	// ReceiverAccount is the account number of the receiver
	ReceiverAccount string `json:"receiver_account"`
	// Amount is the transaction amount in ETB
	Amount float64 `json:"amount"`
	// Date is the payment date as a string
	Date string `json:"date"`
	// TransactionID is the reference number from the receipt
	TransactionID string `json:"transaction_id"`
	// Reason is the payment reason/description
	Reason string `json:"reason"`
}

// VerificationResult represents the result of a transaction verification
type VerificationResult struct {
	// IsValid indicates whether the transaction was successfully verified
	IsValid bool `json:"is_valid"`
	// Details contains the official transaction details if IncludeDetails was true
	Details *TransactionDetails `json:"details,omitempty"`
	// Error contains the error message if verification failed
	Error string `json:"error,omitempty"`
	// Mismatches contains specific field mismatches if verification failed
	Mismatches map[string]interface{} `json:"mismatches,omitempty"`
}

// Verify fetches the official CBE receipt and verifies the provided transaction data
//
// This function:
// 1. Constructs the full transaction ID from the provided ID and suffix
// 2. Fetches the official PDF receipt from CBE servers
// 3. Parses the PDF to extract transaction details
// 4. Compares the provided data with the official records
// 5. Returns a verification result
//
// Example:
//
//	result, err := cbeverifier.Verify(cbeverifier.Transaction{
//		ID:     "xxxxxxxxx",
//		Suffix: "xxxxx",
//		Amount: xxxx.xx,
//	}, cbeverifier.DefaultOptions())
func Verify(transaction Transaction, opts Options) (*VerificationResult, error) {
	// Validate input
	if err := validateTransaction(transaction); err != nil {
		return &VerificationResult{
			IsValid: false,
			Error:   err.Error(),
		}, nil
	}

	// Set default timeout if not specified
	if opts.Timeout <= 0 {
		opts.Timeout = 120
	}

	// Fetch and parse the official receipt
	details, err := fetchAndParseReceipt(transaction.ID, transaction.Suffix, opts)
	if err != nil {
		return &VerificationResult{
			IsValid: false,
			Error:   err.Error(),
		}, nil
	}

	// Compare provided data with official data
	isValid, mismatches := compareTransaction(transaction, details)

	if !isValid {
		return &VerificationResult{
			IsValid:    false,
			Error:      "transaction verification failed",
			Mismatches: mismatches,
		}, nil
	}

	result := &VerificationResult{
		IsValid: true,
	}

	// Include details if requested
	if opts.IncludeDetails {
		result.Details = details
	}

	return result, nil
}

// validateTransaction validates the provided transaction data
func validateTransaction(t Transaction) error {
	if strings.TrimSpace(t.ID) == "" {
		return ErrInvalidTransactionID
	}
	if strings.TrimSpace(t.Suffix) == "" {
		return ErrInvalidSuffix
	}
	if t.Amount <= 0 {
		return ErrInvalidAmount
	}
	return nil
}

// fetchAndParseReceipt fetches the official CBE receipt and parses it
func fetchAndParseReceipt(reference, suffix string, opts Options) (*TransactionDetails, error) {
	fullID := reference + suffix
	url := fmt.Sprintf("https://apps.cbe.com.et:100/?id=%s", fullID)

	// Create HTTP client with custom timeout and TLS config
	client := &http.Client{
		Timeout: time.Duration(opts.Timeout) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Note: This is required for CBE's server
			},
		},
	}

	// Create request with proper headers
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNetworkError, err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (CBE-Verifier-Go/1.0)")
	req.Header.Set("Accept", "application/pdf")
	req.Header.Set("Accept-Encoding", "identity")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNetworkError, err)
	}
	defer resp.Body.Close()

	// Validate response
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if resp.StatusCode != 200 || !strings.Contains(contentType, "application/pdf") {
		return nil, ErrInvalidPDFResponse
	}

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPDFReadError, err)
	}

	// Parse the PDF
	result := ParseCBEReceipt(bodyBytes)
	if !result.Success {
		return nil, fmt.Errorf("%w: %v", ErrReceiptParseError, result.Details["error"])
	}

	// Convert to TransactionDetails
	details := &TransactionDetails{
		Payer:           getString(result.Details, "payer"),
		PayerAccount:    getString(result.Details, "payerAccount"),
		Receiver:        getString(result.Details, "receiver"),
		ReceiverAccount: getString(result.Details, "receiverAccount"),
		Amount:          getFloat64(result.Details, "amount"),
		Date:            getString(result.Details, "date"),
		TransactionID:   getString(result.Details, "transaction_id"),
		Reason:          getString(result.Details, "reason"),
	}

	return details, nil
}

// compareTransaction compares provided transaction data with official details
func compareTransaction(provided Transaction, official *TransactionDetails) (bool, map[string]interface{}) {
	mismatches := make(map[string]interface{})

	// Compare transaction ID
	providedID := strings.TrimSpace(provided.ID)
	officialID := strings.TrimSpace(official.TransactionID)
	if providedID != officialID {
		mismatches["transaction_id"] = map[string]interface{}{
			"provided": providedID,
			"official": officialID,
		}
	}

	// Compare amount (with rounding to handle floating point precision)
	if round2(provided.Amount) != round2(official.Amount) {
		mismatches["amount"] = map[string]interface{}{
			"provided": provided.Amount,
			"official": official.Amount,
		}
	}

	return len(mismatches) == 0, mismatches
}

// Helper functions
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getFloat64(m map[string]interface{}, key string) float64 {
	if val, ok := m[key]; ok {
		if f, ok := val.(float64); ok {
			return f
		}
		// Try to convert from string
		if str, ok := val.(string); ok {
			if f, err := strconv.ParseFloat(str, 64); err == nil {
				return f
			}
		}
	}
	return 0
}

func round2(val float64) float64 {
	return float64(int(val*100+0.5)) / 100
}

