package decimal_test

import (
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/jacoelho/decimal"
)

func TestParseValidInputs(t *testing.T) {
	maxDecimal := decimal.Max().String()
	tests := []struct {
		input string
		want  string
	}{
		{"0", "0"},
		{"-0", "0"},
		{"123", "123"},
		{"-123", "-123"},
		{"123.4500", "123.4500"},
		{"0.00123", "0.00123"},
		{".01", "0.01"},
		{"+.01", "0.01"},
		{"-.01", "-0.01"},
		{"-.125", "-0.125"},
		{"1.", "1"},
		{"1e3", "1000"},
		{"1.23e2", "123"},
		{"1.23e-2", "0.0123"},
		{"1.0e-28", "0.0000000000000000000000000001"},
		{"1.00000000000000000000000000000000", "1." + strings.Repeat("0", int(decimal.MaxScale))},
		{"1." + strings.Repeat("0", 80), "1." + strings.Repeat("0", int(decimal.MaxScale))},
		{"1" + strings.Repeat("0", 80) + "e-80", "1." + strings.Repeat("0", int(decimal.MaxScale))},
		{maxDecimal + ".0", maxDecimal},
		{"-" + maxDecimal + ".0", "-" + maxDecimal},
		{maxDecimal + "0e-1", maxDecimal},
		{"0e100", "0"},
		{"-0e-100", "0"},
		{"0e-9223372036854775808", "0"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := decimal.Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if got.String() != tt.want {
				t.Fatalf("Parse(%q).String() = %q, want %q", tt.input, got.String(), tt.want)
			}
		})
	}
}

func TestParseRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		input string
		want  error
	}{
		{"", decimal.ErrParse},
		{"-", decimal.ErrParse},
		{"1.2.3", decimal.ErrParse},
		{"1e", decimal.ErrParse},
		{"1e-29", decimal.ErrInvalid},
		{"1e-9223372036854775808", decimal.ErrInvalid},
		{"999999999999999999999999999999999999999", decimal.ErrOverflow},
		{"1e999999999999999999999999", decimal.ErrParse},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := decimal.Parse(tt.input)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Parse(%q) error = %v, want %v", tt.input, err, tt.want)
			}
			if !errors.Is(err, decimal.ErrParse) {
				t.Fatalf("Parse(%q) error = %v, want ErrParse", tt.input, err)
			}
			if strings.Contains(err.Error(), "\n") {
				t.Fatalf("Parse(%q) error contains newline: %q", tt.input, err)
			}
		})
	}
}

func FuzzParseRoundTrip(f *testing.F) {
	for _, seed := range []string{
		"0",
		"-0",
		"123.4500",
		".01",
		"+.01",
		"-.01",
		"1e28",
		"1e-28",
		"79228162514264337593543950335",
		"79228162514264337593543950335.0",
		"0e-9223372036854775808",
		"1.2.3",
		"1e",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 512 {
			return
		}

		d, err := decimal.Parse(s)
		if err != nil {
			if !errors.Is(err, decimal.ErrParse) {
				t.Fatalf("Parse(%q) error = %v, want ErrParse wrapper", s, err)
			}
			if strings.Contains(err.Error(), "\n") {
				t.Fatalf("Parse(%q) error contains newline: %q", s, err)
			}
			return
		}

		roundTrip, err := decimal.Parse(d.String())
		if err != nil {
			t.Fatalf("Parse(String()) failed for %q -> %q: %v", s, d.String(), err)
		}
		if !roundTrip.Equal(d) {
			t.Fatalf("Parse(String()) = %s, want %s from input %q", roundTrip, d, s)
		}

		want, ok := new(big.Rat).SetString(s)
		if !ok {
			if !d.IsZero() {
				t.Fatalf("Parse(%q) accepted non-zero %s but big.Rat rejected input", s, d)
			}
			return
		}
		if got := decimalRat(d); got.Cmp(want) != 0 {
			t.Fatalf("Parse(%q) = %s (%s), want %s", s, d, got, want)
		}
	})
}
