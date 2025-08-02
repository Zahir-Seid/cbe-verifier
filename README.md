# CBE Verifier Go Library

A Go library for verifying Commercial Bank of Ethiopia (CBE) transaction receipts by fetching and parsing official PDF documents from the bank's servers.

## Features

- 🔍 **Transaction Verification**: Verify transaction details against official CBE records
- 📄 **PDF Parsing**: Automatically parse CBE receipt PDFs to extract transaction information
- 🛡️ **Error Handling**: Comprehensive error handling with detailed mismatch information
- ⚡ **Configurable**: Customizable timeouts and verification settings
- 📦 **Library Ready**: Designed as a reusable Go library with clean API

## Installation

```bash
go get github.com/yourusername/cbe-verifier
```

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    "github.com/Zahir-Seid/cbe-verifier/cbeverifier"
)

func main() {
    // Create transaction data to verify
    transaction := cbeverifier.Transaction{
        ID:     "xxxxx",
        Suffix: "xxxxx",
        Amount: xxx.xx,
    }

    // Verify against official records
    result, err := cbeverifier.Verify(transaction, cbeverifier.DefaultOptions())
    if err != nil {
        log.Fatal(err)
    }

    if result.IsValid {
        fmt.Println("Transaction verified successfully!")
    } else {
        fmt.Printf("Verification failed: %s\n", result.Error)
    }
}
```

## API Reference

### Types

#### Transaction
Represents a CBE transaction to be verified.

```go
type Transaction struct {
    ID     string  `json:"id"`     // Transaction reference number (e.g., "xxxxx")
    Suffix string  `json:"suffix"` // Transaction suffix (e.g., "xxxxx")
    Amount float64 `json:"amount"` // Transaction amount in ETB
}
```

#### Options
Configures the verification process.

```go
type Options struct {
    IncludeDetails bool   `json:"include_details"` // Return full transaction details
    Timeout        int    `json:"timeout"`         // HTTP timeout in seconds (default: 120)
}
```

#### VerificationResult
Result of a transaction verification.

```go
type VerificationResult struct {
    IsValid    bool                    `json:"is_valid"`    // Whether verification succeeded
    Details    *TransactionDetails     `json:"details"`     // Official transaction details
    Error      string                  `json:"error"`       // Error message if failed
    Mismatches map[string]interface{}  `json:"mismatches"`  // Field mismatches if failed
}
```

#### TransactionDetails
Parsed transaction information from the official receipt.

```go
type TransactionDetails struct {
    Payer           string  `json:"payer"`            // Payer name
    PayerAccount    string  `json:"payer_account"`    // Payer account number
    Receiver        string  `json:"receiver"`         // Receiver name
    ReceiverAccount string  `json:"receiver_account"` // Receiver account number
    Amount          float64 `json:"amount"`           // Transaction amount
    Date            string  `json:"date"`             // Payment date
    TransactionID   string  `json:"transaction_id"`   // Reference number
    Reason          string  `json:"reason"`           // Payment reason
}
```

### Functions

#### Verify
Main function to verify a transaction against official CBE records.

```go
func Verify(transaction Transaction, opts Options) (*VerificationResult, error)
```

**Parameters:**
- `transaction`: Transaction data to verify
- `opts`: Verification options

**Returns:**
- `*VerificationResult`: Verification result
- `error`: Error if verification process failed

#### DefaultOptions
Returns default verification options.

```go
func DefaultOptions() Options
```

#### ParseCBEReceipt
Parse a CBE receipt PDF and extract transaction information.

```go
func ParseCBEReceipt(pdfBytes []byte) VerifyResult
```

**Parameters:**
- `pdfBytes`: PDF file content as bytes

**Returns:**
- `VerifyResult`: Parsing result with extracted details or error information

## Usage Examples

### Basic Verification

```go
transaction := cbeverifier.Transaction{
    ID:     "xxxxx",
    Suffix: "xxxxx",
    Amount: xxx.xx,
}

result, err := cbeverifier.Verify(transaction, cbeverifier.DefaultOptions())
if err != nil {
    log.Fatal(err)
}

if result.IsValid {
    fmt.Println("Transaction verified successfully!")
} else {
    fmt.Printf("Verification failed: %s\n", result.Error)
}
```

### Verification with Full Details

```go
opts := cbeverifier.Options{
    IncludeDetails: true,
    Timeout:        120,
}

result, err := cbeverifier.Verify(transaction, opts)
if err != nil {
    log.Fatal(err)
}

if result.IsValid && result.Details != nil {
    fmt.Printf("Amount: %.2f ETB\n", result.Details.Amount)
    fmt.Printf("Payer: %s\n", result.Details.Payer)
    fmt.Printf("Receiver: %s\n", result.Details.Receiver)
    fmt.Printf("Date: %s\n", result.Details.Date)
}
```

### Error Handling

```go
result, err := cbeverifier.Verify(transaction, cbeverifier.DefaultOptions())
if err != nil {
    log.Printf("Verification process failed: %v", err)
    return
}

if !result.IsValid {
    fmt.Printf("Verification failed: %s\n", result.Error)
    
    // Check for specific mismatches
    if result.Mismatches != nil {
        for field, mismatch := range result.Mismatches {
            if mismatchMap, ok := mismatch.(map[string]interface{}); ok {
                fmt.Printf("Field %s: provided=%v, official=%v\n",
                    field, mismatchMap["provided"], mismatchMap["official"])
            }
        }
    }
}
```

### PDF Parsing Only

```go
// Read PDF file
pdfBytes, err := os.ReadFile("receipt.pdf")
if err != nil {
    log.Fatal(err)
}

// Parse PDF
result := cbeverifier.ParseCBEReceipt(pdfBytes)
if result.Success {
    fmt.Printf("Amount: %.2f ETB\n", result.Details["amount"])
    fmt.Printf("Payer: %s\n", result.Details["payer"])
} else {
    fmt.Printf("Parse error: %v\n", result.Details["error"])
}
```

## Error Handling

The library provides comprehensive error handling with specific error types:

- `ErrInvalidTransactionID`: Invalid transaction ID
- `ErrInvalidSuffix`: Invalid suffix
- `ErrInvalidAmount`: Invalid amount
- `ErrNetworkError`: Network communication error
- `ErrInvalidPDFResponse`: Invalid PDF response from CBE
- `ErrPDFReadError`: PDF content read error
- `ErrReceiptParseError`: PDF parsing error
- `ErrVerificationFailed`: Transaction verification failed

## Configuration

### Timeout Settings

```go
opts := cbeverifier.Options{
    Timeout: 60, // 60 seconds timeout
}
```
## Dependencies

- `github.com/dslipak/pdf`: PDF parsing library

## Requirements

- Go 1.24 or later
- Internet connection for fetching CBE receipts

## Security Notes

- The library uses `InsecureSkipVerify: true` for TLS connections to CBE servers as required by their certificate configuration
- No sensitive data is logged or stored unless explicitly configured

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Disclaimer

This library is not officially affiliated with the Commercial Bank of Ethiopia. Use at your own risk and ensure compliance with CBE's terms of service. 