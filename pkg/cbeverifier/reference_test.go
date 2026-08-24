package cbeverifier

import (
	"testing"
)

func TestClassifyReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    RefKind
		wantID  string
		wantSuf string
		wantTok string
	}{
		{name: "bare legacy reference", input: testLegacyReference, want: RefLegacy, wantID: testLegacyReference},
		{name: "bare legacy lowercased", input: "ft25062pp5zb", want: RefLegacy, wantID: testLegacyReference},
		{name: "legacy with surrounding spaces", input: "  FT25062PP5ZB\n", want: RefLegacy, wantID: testLegacyReference},
		{
			name:    "legacy URL with embedded suffix",
			input:   "https://apps.cbe.com.et:100/?id=FT25062PP5ZB87654321",
			want:    RefLegacyURL,
			wantID:  testLegacyReference,
			wantSuf: "87654321",
		},
		{
			name:    "legacy http URL",
			input:   "http://apps.cbe.com.et:100/?id=FT25062PP5ZB11112222",
			want:    RefLegacyURL,
			wantID:  testLegacyReference,
			wantSuf: "11112222",
		},
		{
			name:    "legacy URL without explicit port",
			input:   "https://apps.cbe.com.et/?id=FT25062PP5ZB99998888",
			want:    RefLegacyURL,
			wantID:  testLegacyReference,
			wantSuf: "99998888",
		},
		{name: "legacy URL wrong host", input: "https://apps.cbe.com.evil/?id=FT25062PP5ZB12345678", want: RefUnknown},
		{name: "legacy URL wrong port", input: "https://apps.cbe.com.et:8080/?id=FT25062PP5ZB12345678", want: RefUnknown},
		{name: "legacy URL missing id", input: "https://apps.cbe.com.et:100/", want: RefUnknown},
		{name: "legacy URL short suffix", input: "https://apps.cbe.com.et:100/?id=FT25062PP5ZB1234567", want: RefUnknown},
		{name: "combined id without URL is unknown (parity with TS)", input: "FT25062PP5ZB12345678", want: RefUnknown},
		{name: "plain token", input: testNewToken, want: RefNewToken, wantTok: testNewToken},
		{name: "token with hyphens at max length", input: "AbCdEf123-456-789-AbCdEf123-4567", want: RefNewToken, wantTok: "AbCdEf123-456-789-AbCdEf123-4567"},
		{name: "mbreciept URL", input: "https://mbreciept.cbe.com.et/AbCdEf123456789", want: RefNewToken, wantTok: testNewToken},
		{name: "mbreciept URL trailing slash", input: "https://mbreciept.cbe.com.et/AbCdEf123456789/", want: RefNewToken, wantTok: testNewToken},
		{name: "token too short", input: "Ab1", want: RefUnknown},
		{name: "token too long", input: "AbCdEf123456789AbCdEf123456789AbCdEf123456X", want: RefUnknown},
		{name: "FT-prefixed string is never a new token", input: "FT25062PP5ZBEXTRA17", want: RefUnknown},
		{name: "empty string", input: "", want: RefUnknown},
		{name: "random text", input: "hello world this is not a ref", want: RefUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ClassifyReference(tt.input)
			if got.Kind != tt.want {
				t.Fatalf("Kind = %v, want %v", got.Kind, tt.want)
			}

			if got.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", got.ID, tt.wantID)
			}

			if got.Suffix != tt.wantSuf {
				t.Errorf("Suffix = %q, want %q", got.Suffix, tt.wantSuf)
			}

			if got.Token != tt.wantTok {
				t.Errorf("Token = %q, want %q", got.Token, tt.wantTok)
			}
		})
	}
}

func FuzzClassifyReference(f *testing.F) {
	f.Add(testLegacyReference)
	f.Add("https://apps.cbe.com.et:100/?id=FT25062PP5ZB12345678")
	f.Add("https://mbreciept.cbe.com.et/AbCdEf123456789")
	f.Add("")
	f.Add("%%%%")

	f.Fuzz(func(t *testing.T, input string) {
		got := ClassifyReference(input)
		switch got.Kind {
		case RefUnknown:
			// Nothing further to assert for unrecognized shapes.
		case RefLegacy:
			if len(got.ID) != 12 {
				t.Errorf("legacy ID %q has length %d, want 12", got.ID, len(got.ID))
			}
		case RefLegacyURL:
			if len(got.Suffix) != 8 {
				t.Errorf("URL suffix %q has length %d, want 8", got.Suffix, len(got.Suffix))
			}

			if got.Suffix == "" || got.ID == "" {
				t.Error("legacy URL classification missing ID or suffix")
			}
		case RefNewToken:
			if got.Token == "" {
				t.Error("new-token classification with empty token")
			}
		}
	})
}
