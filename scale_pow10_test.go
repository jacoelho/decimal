package decimal_test

import (
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"strings"
	"testing"

	"github.com/jacoelho/decimal"
)

func TestScalePow10(t *testing.T) {
	const maxInt = int(^uint(0) >> 1)
	const minInt = -maxInt - 1
	smallest := decimal.MustNew(decimal.MantissaFromUint64(1), 28, false)
	tenAtMaxScale := decimal.MustNew(decimal.MantissaFromUint64(10), 28, false)
	hundredAtMaxScale := decimal.MustNew(decimal.MantissaFromUint64(100), 28, false)
	elevenAtMaxScale := decimal.MustNew(decimal.MantissaFromUint64(11), 28, false)
	negative := mustParse(t, "-1.2300")

	tests := []struct {
		name    string
		input   decimal.Decimal
		n       int
		want    decimal.Decimal
		wantErr error
	}{
		{name: "right", input: mustParse(t, "1.2300"), n: 2, want: mustParse(t, "123.00")},
		{name: "left", input: mustParse(t, "1.2300"), n: -2, want: mustParse(t, "0.012300")},
		{name: "negative right", input: negative, n: 2, want: mustParse(t, "-123.00")},
		{name: "negative left", input: negative, n: -2, want: mustParse(t, "-0.012300")},
		{name: "identity", input: negative, want: negative},
		{name: "integer multiplication", input: mustParse(t, "1.2300"), n: 5, want: decimal.FromUint64(123000)},
		{name: "remove necessary zero", input: tenAtMaxScale, n: -1, want: smallest},
		{name: "retain remaining zero", input: hundredAtMaxScale, n: -1, want: tenAtMaxScale},
		{name: "nonzero discarded digit", input: elevenAtMaxScale, n: -1, wantErr: decimal.ErrInexact},
		{name: "largest right shift", input: smallest, n: 56, want: mustParse(t, "1e28")},
		{name: "largest left shift", input: mustParse(t, "1e28"), n: -56, want: smallest},
		{name: "underflow", input: smallest, n: -1, wantErr: decimal.ErrInexact},
		{name: "overflow", input: decimal.Max(), n: 1, wantErr: decimal.ErrOverflow},
		{name: "negative overflow", input: decimal.Min(), n: 1, wantErr: decimal.ErrOverflow},
		{name: "multiplication boundary fits", input: mustParse(t, "7922816251426433759354395033"), n: 1, want: mustParse(t, "79228162514264337593543950330")},
		{name: "multiplication boundary overflows", input: mustParse(t, "7922816251426433759354395034"), n: 1, wantErr: decimal.ErrOverflow},
		{name: "zero identity"},
		{name: "zero minimum exponent", n: minInt},
		{name: "zero maximum exponent", n: maxInt},
		{name: "positive minimum exponent", input: decimal.FromInt64(1), n: minInt, wantErr: decimal.ErrInexact},
		{name: "positive maximum exponent", input: decimal.FromInt64(1), n: maxInt, wantErr: decimal.ErrOverflow},
		{name: "negative minimum exponent", input: decimal.FromInt64(-1), n: minInt, wantErr: decimal.ErrInexact},
		{name: "negative maximum exponent", input: decimal.FromInt64(-1), n: maxInt, wantErr: decimal.ErrOverflow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.input.ScalePow10(tt.n)
			if !errors.Is(err, tt.wantErr) || decimalBits(got) != decimalBits(tt.want) {
				t.Fatalf("%s.ScalePow10(%d) = %s bits=%x err=%v, want %s bits=%x err=%v",
					tt.input, tt.n, got, decimalBits(got), err, tt.want, decimalBits(tt.want), tt.wantErr)
			}
			if tt.wantErr != nil && !strings.Contains(err.Error(), fmt.Sprintf("decimal: scale pow10: exponent %d:", tt.n)) {
				t.Fatalf("error %q lacks operation and exponent context", err)
			}
		})
	}
}

func TestScalePow10MatchesBigRat(t *testing.T) {
	rng := rand.New(rand.NewSource(3000)) //nolint:gosec // Deterministic property data.
	for i := range 2048 {
		d := randomScalePow10Decimal(t, rng, i%4)
		s := int(d.Scale())
		n := rng.Intn(115) - 57
		if rng.Intn(2) == 0 {
			boundaries := [...]int{-57, -56, -29, -28, -1, 0, 1, 28, 29, 56, 57, s - 29, s - 28, s, s + 1}
			n = boundaries[rng.Intn(len(boundaries))]
		}

		want := decimalRat(d)
		if n >= 0 {
			want.Mul(want, new(big.Rat).SetInt(pow10Big(n)))
		} else {
			want.Quo(want, new(big.Rat).SetInt(pow10Big(-n)))
		}
		_, wantErr := exactDecimalOracle(want)
		got, err := d.ScalePow10(n)
		if !errors.Is(err, wantErr) ||
			wantErr == nil && decimalRat(got).Cmp(want) != 0 ||
			wantErr != nil && decimalBits(got) != [4]uint32{} {
			t.Fatalf("case %d seed=3000: input bits=%x scale=%d exponent=%d: got %s bits=%x err=%v; want %s err=%v",
				i, decimalBits(d), d.Scale(), n, got, decimalBits(got), err, want, wantErr)
		}
	}
}

func randomScalePow10Decimal(t *testing.T, rng *rand.Rand, family int) decimal.Decimal {
	t.Helper()
	var m *big.Int
	switch family {
	case 0:
		m = mantissaBig(decimal.NewMantissa(rng.Uint32(), rng.Uint64()))
	case 1:
		m = new(big.Int).SetUint64(rng.Uint64() % 1_000_000)
	case 2:
		m = new(big.Int).Mul(big.NewInt(int64(1+rng.Intn(7))), pow10Big(rng.Intn(29)))
	default:
		m = new(big.Int).Sub(maxMantissaBig(), new(big.Int).SetUint64(rng.Uint64()%1000))
	}
	d, ok := decimalFromBigMantissa(m, randomScale(rng), rng.Intn(2) == 0)
	if !ok {
		t.Fatalf("generated mantissa %s cannot fit in Decimal", m)
	}
	return d
}

func TestExactDecimalOracleBoundaries(t *testing.T) {
	tests := []struct {
		rational string
		wantErr  error
	}{
		{"0", nil},
		{"123/100", nil},
		{"-123/100", nil},
		{"79228162514264337593543950335", nil},
		{"-79228162514264337593543950335", nil},
		{"79228162514264337593543950336", decimal.ErrOverflow},
		{"-79228162514264337593543950336", decimal.ErrOverflow},
		{"1/10000000000000000000000000000", nil},
		{"1/100000000000000000000000000000", decimal.ErrInexact},
		{"11/100000000000000000000000000000", decimal.ErrInexact},
		{"1/3", decimal.ErrInexact},
	}
	for _, tt := range tests {
		t.Run(tt.rational, func(t *testing.T) {
			r, ok := new(big.Rat).SetString(tt.rational)
			if !ok {
				t.Fatalf("invalid rational fixture %q", tt.rational)
			}
			got, err := exactDecimalOracle(r)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("oracle(%s) error = %v, want %v", r, err, tt.wantErr)
			}
			if err == nil && decimalRat(got).Cmp(r) != 0 {
				t.Fatalf("oracle(%s) = %s", r, got)
			}
		})
	}
}

func TestDivPowerOfTenScaleAndSign(t *testing.T) {
	for _, tt := range []struct {
		divisor string
		want    string
	}{
		{"10.00", "0.123"},
		{"0.01", "123"},
		{"1.00", "1.23"},
	} {
		for _, dividendNegative := range []bool{false, true} {
			for _, divisorNegative := range []bool{false, true} {
				a := mustParse(t, "1.2300")
				b := mustParse(t, tt.divisor)
				if dividendNegative {
					a = a.Neg()
				}
				if divisorNegative {
					b = b.Neg()
				}
				want := tt.want
				if dividendNegative != divisorNegative {
					want = "-" + want
				}
				t.Run(a.String()+"/"+b.String(), func(t *testing.T) {
					got, err := a.Div(b)
					if err != nil || got.String() != want {
						t.Fatalf("Div = %s err=%v, want %s", got, err, want)
					}
				})
			}
		}
	}
}

func TestDivPowerOfTenErrorContext(t *testing.T) {
	for _, tt := range []struct {
		name    string
		input   decimal.Decimal
		divisor decimal.Decimal
		wantErr error
	}{
		{"overflow", decimal.Max(), mustParse(t, "0.1"), decimal.ErrOverflow},
		{"inexact", mustParse(t, "1e-28"), decimal.FromUint64(10), decimal.ErrInexact},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.input.Div(tt.divisor)
			if !errors.Is(err, tt.wantErr) || decimalBits(got) != [4]uint32{} {
				t.Fatalf("Div = %s err=%v, want canonical zero and %v", got, err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), "decimal: div:") {
				t.Fatalf("error %q lacks div context", err)
			}
		})
	}
}

func TestScalePow10DoesNotAllocate(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input decimal.Decimal
		n     int
		want  decimal.Decimal
	}{
		{name: "zero", n: 56},
		{name: "scale only", input: mustParse(t, "1.2300"), n: 2, want: mustParse(t, "123.00")},
		{name: "multiply", input: mustParse(t, "1.2300"), n: 5, want: decimal.FromUint64(123000)},
		{name: "remove zero", input: decimal.MustNew(decimal.MantissaFromUint64(10), 28, false), n: -1, want: mustParse(t, "1e-28")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got decimal.Decimal
			var err error
			allocs := testing.AllocsPerRun(100, func() {
				got, err = tt.input.ScalePow10(tt.n)
			})
			if allocs != 0 {
				t.Errorf("ScalePow10 allocated %g objects per call, want 0", allocs)
			}
			if err != nil || decimalBits(got) != decimalBits(tt.want) {
				t.Fatalf("ScalePow10 = %s bits=%x err=%v, want %s bits=%x", got, decimalBits(got), err, tt.want, decimalBits(tt.want))
			}
		})
	}
}

func decimalBits(d decimal.Decimal) [4]uint32 {
	lo, mid, hi, flags := d.Bits()
	return [4]uint32{lo, mid, hi, flags}
}
