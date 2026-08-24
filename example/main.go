// Command cbeverify verifies a Commercial Bank of Ethiopia payment receipt
// against official CBE records.
//
// The reference may be a bare legacy reference, a full receipt URL from
// either platform, or a new-platform token; the backend is selected
// automatically.
//
//	go run ./example --reference FT25062PP5ZB --suffix 12345678 --amount 1500
//	go run ./example --reference "https://mbreciept.cbe.com.et/AbCdEf123456789" --amount 250
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	cbeverifier "github.com/Zahir-Seid/cbe-verifier/v2/pkg/cbeverifier"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("cbeverify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	reference := fs.String("reference", "", "payment reference, receipt URL, or new-platform token (required)")
	suffix := fs.String("suffix", "", "payer account suffix; required for bare legacy references")
	amount := fs.Float64("amount", 0, "expected amount in ETB (required)")
	timeout := fs.Duration("timeout", cbeverifier.DefaultTimeout, "per-request HTTP timeout")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := validateFlags(reference, amount); err != nil {
		fmt.Fprintln(stderr, err)
		fs.Usage()
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout+30*time.Second)
	defer cancel()

	result, err := cbeverifier.Verify(ctx, cbeverifier.Transaction{
		Reference: *reference,
		Suffix:    *suffix,
		Amount:    *amount,
	}, cbeverifier.WithTimeout(*timeout))
	if err != nil {
		fmt.Fprintf(stderr, "cbeverify: %v\n", err)
		return 2
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintf(stderr, "cbeverify: encode output: %v\n", err)
		return 2
	}
	if !result.Valid {
		return 1
	}
	return 0
}

func validateFlags(reference *string, amount *float64) error {
	if *reference == "" {
		return errors.New("cbeverify: -reference is required")
	}
	if *amount <= 0 {
		return errors.New("cbeverify: -amount must be greater than zero")
	}
	return nil
}
