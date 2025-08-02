// Package main provides a command-line interface to verify CBE transactions
// go run main.go --id=xxxxxxx --suffix=xxxxxxx --amount=xxxx.xx
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Zahir-Seid/cbe-verifier/cbeverifier"
)

func main() {
	// Define CLI flags
	id := flag.String("id", "", "Transaction reference ID (e.g., FTxxxxxxxxx)")
	suffix := flag.String("suffix", "", "Transaction suffix (e.g., xxxxxxxx)") // Account suffix is the number after 1000 in CBE aacounts
	amount := flag.Float64("amount", 0.0, "Transaction amount in ETB (e.g., xxxx.xx)")
	includeDetails := flag.Bool("details", true, "Include full transaction details")

	flag.Parse()

	// Validate required fields
	if *id == "" || *suffix == "" || *amount <= 0 {
		fmt.Fprintln(os.Stderr, "Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Construct transaction
	transaction := cbeverifier.Transaction{
		ID:     *id,
		Suffix: *suffix,
		Amount: *amount,
	}

	// Options
	options := cbeverifier.Options{
		IncludeDetails: *includeDetails,
		Timeout:        120,
	}

	// Verify transaction
	result, err := cbeverifier.Verify(transaction, options)
	if err != nil {
		log.Fatalf("Verification error: %v\n", err)
	}

	if result.IsValid {
		fmt.Println("Transaction verified successfully.")
		if result.Details != nil {
			fmt.Printf("Amount: %.2f ETB\n", result.Details.Amount)
			fmt.Printf("Payer: %s\n", result.Details.Payer)
			fmt.Printf("Receiver: %s\n", result.Details.Receiver)
			fmt.Printf("Date: %s\n", result.Details.Date)
			fmt.Printf("Reason: %s\n", result.Details.Reason)
		}
	} else {
		fmt.Printf(" Verification failed: %s\n", result.Error)
		if result.Mismatches != nil {
			fmt.Println("Mismatches:")
			for field, mismatch := range result.Mismatches {
				fmt.Printf("  - %s: %v\n", field, mismatch)
			}
		}
	}
}
