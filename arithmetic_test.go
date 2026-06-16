package decimal_test

import (
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"github.com/jacoelho/decimal"
)

type quickDecimal struct {
	decimal.Decimal
}

func (quickDecimal) Generate(r *rand.Rand, _ int) reflect.Value {
	return reflect.ValueOf(quickDecimal{Decimal: randomDecimal(r)})
}

func TestExactArithmetic(t *testing.T) {
	tests := []struct {
		name string
		run  func() (decimal.Decimal, error)
		want string
	}{
		{
			name: "add aligns scale exactly",
			run: func() (decimal.Decimal, error) {
				return mustParse(t, "1.2").Add(mustParse(t, "0.03"))
			},
			want: "1.23",
		},
		{
			name: "sub borrows across aligned fractional scale",
			run: func() (decimal.Decimal, error) {
				return mustParse(t, "1.00").Sub(mustParse(t, "0.001"))
			},
			want: "0.999",
		},
		{
			name: "mul preserves exact result scale",
			run: func() (decimal.Decimal, error) {
				return mustParse(t, "-12.5").Mul(mustParse(t, "2.00"))
			},
			want: "-25.000",
		},
		{
			name: "div returns terminating quotient",
			run: func() (decimal.Decimal, error) {
				return decimal.FromUint64(1).Div(decimal.FromUint64(8))
			},
			want: "0.125",
		},
		{
			name: "div by power of ten shifts scale",
			run: func() (decimal.Decimal, error) {
				return decimal.FromUint64(1).Div(decimal.FromUint64(10))
			},
			want: "0.1",
		},
		{
			name: "div max-scale power of ten stays exact when trailing zero is available",
			run: func() (decimal.Decimal, error) {
				return decimal.MustNew(decimal.MantissaFromUint64(10), decimal.MaxScale, false).Div(decimal.FromUint64(10))
			},
			want: "0." + strings.Repeat("0", int(decimal.MaxScale)-1) + "1",
		},
		{
			name: "div normalizes exact quotient",
			run: func() (decimal.Decimal, error) {
				return mustParse(t, "1.00").Div(mustParse(t, "0.25"))
			},
			want: "4",
		},
		{
			name: "div by negative one normalizes and flips sign",
			run: func() (decimal.Decimal, error) {
				return mustParse(t, "1.20").Div(decimal.FromInt64(-1))
			},
			want: "-1.2",
		},
		{
			name: "remainder keeps dividend sign",
			run: func() (decimal.Decimal, error) {
				return mustParse(t, "-5.5").Remainder(decimal.FromUint64(2))
			},
			want: "-1.5",
		},
		{
			name: "remainder smaller than divisor uses common scale",
			run: func() (decimal.Decimal, error) {
				return mustParse(t, "1.2").Remainder(mustParse(t, "2.00"))
			},
			want: "1.20",
		},
		{
			name: "remainder nearby larger divisor uses common scale",
			run: func() (decimal.Decimal, error) {
				return mustParse(t, "1.2").Remainder(mustParse(t, "1.23"))
			},
			want: "1.20",
		},
		{
			name: "remainder handles wide common scale",
			run: func() (decimal.Decimal, error) {
				return decimal.Max().Remainder(decimal.MustNew(decimal.MantissaFromUint64(1), decimal.MaxScale, false))
			},
			want: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.run()
			if err != nil {
				t.Fatalf("operation failed: %v", err)
			}
			if got.String() != tt.want {
				t.Fatalf("operation = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestExactArithmeticErrors(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{
			name: "add overflows above max",
			run: func() error {
				_, err := decimal.Max().Add(mustParse(t, "0.1"))
				return err
			},
			want: decimal.ErrOverflow,
		},
		{
			name: "sub cannot represent fractional max result",
			run: func() error {
				_, err := decimal.Max().Sub(mustParse(t, "0.1"))
				return err
			},
			want: decimal.ErrInexact,
		},
		{
			name: "mul reports non-zero underflow as inexact",
			run: func() error {
				_, err := mustParse(t, "0.0000000000000000000000000001").Mul(mustParse(t, "0.0000000000000000000000000001"))
				return err
			},
			want: decimal.ErrInexact,
		},
		{
			name: "mul overflows integer range",
			run: func() error {
				_, err := decimal.Max().Mul(decimal.FromUint64(2))
				return err
			},
			want: decimal.ErrOverflow,
		},
		{
			name: "div rejects repeating quotient",
			run: func() error {
				_, err := decimal.FromUint64(1).Div(decimal.FromUint64(3))
				return err
			},
			want: decimal.ErrInexact,
		},
		{
			name: "div reports quotient above max as overflow",
			run: func() error {
				_, err := maxMinus(7).Div(mustParse(t, "0.9999999999999999999999999999"))
				return err
			},
			want: decimal.ErrOverflow,
		},
		{
			name: "div by power of ten reports max-scale underflow as inexact",
			run: func() error {
				_, err := decimal.MustNew(decimal.MantissaFromUint64(1), decimal.MaxScale, false).Div(decimal.FromUint64(10))
				return err
			},
			want: decimal.ErrInexact,
		},
		{
			name: "div rejects zero divisor",
			run: func() error {
				_, err := decimal.FromUint64(1).Div(decimal.Decimal{})
				return err
			},
			want: decimal.ErrDivideByZero,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSubErrorContext(t *testing.T) {
	_, err := decimal.Min().Sub(decimal.FromUint64(1))
	if !errors.Is(err, decimal.ErrOverflow) {
		t.Fatalf("Sub error = %v, want ErrOverflow", err)
	}
	if !strings.Contains(err.Error(), "decimal: sub:") {
		t.Fatalf("Sub error = %v, want sub context", err)
	}
}

func TestRescaleIsExactOnly(t *testing.T) {
	got, err := mustParse(t, "1.2300").Rescale(2)
	if err != nil {
		t.Fatalf("Rescale exact failed: %v", err)
	}
	if got.String() != "1.23" {
		t.Fatalf("Rescale exact = %s, want 1.23", got)
	}

	if _, err := mustParse(t, "1.234").Rescale(2); !errors.Is(err, decimal.ErrInexact) {
		t.Fatalf("Rescale inexact error = %v, want ErrInexact", err)
	}
}

func TestQuantizeToScale(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		scale uint8
		mode  decimal.RoundingMode
		want  string
	}{
		{"pads fractional zeroes", "1.2", 2, decimal.RoundHalfAwayFromZero, "1.20"},
		{"half away positive", "1.235", 2, decimal.RoundHalfAwayFromZero, "1.24"},
		{"half away negative", "-1.235", 2, decimal.RoundHalfAwayFromZero, "-1.24"},
		{"half even down", "1.245", 2, decimal.RoundHalfToEven, "1.24"},
		{"half even up", "1.255", 2, decimal.RoundHalfToEven, "1.26"},
		{"half toward zero", "1.255", 2, decimal.RoundHalfTowardZero, "1.25"},
		{"away from zero", "-1.231", 2, decimal.RoundAwayFromZero, "-1.24"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mustParse(t, tt.in).Quantize(tt.scale, tt.mode)
			if err != nil {
				t.Fatalf("Quantize failed: %v", err)
			}
			if got.String() != tt.want {
				t.Fatalf("Quantize = %s, want %s", got, tt.want)
			}
			if !got.IsZero() && got.Scale() != tt.scale {
				t.Fatalf("Quantize scale = %d, want %d", got.Scale(), tt.scale)
			}
		})
	}
}

func TestQuantizeToQuantum(t *testing.T) {
	quantum := mustParse(t, "0.050")
	got, err := mustParse(t, "1.275").QuantizeTo(quantum, decimal.RoundHalfToEven)
	if err != nil {
		t.Fatalf("QuantizeTo failed: %v", err)
	}
	if got.String() != "1.300" {
		t.Fatalf("QuantizeTo = %s, want 1.300", got)
	}

	got, err = mustParse(t, "1.27").QuantizeTo(mustParse(t, "0.05"), decimal.RoundHalfAwayFromZero)
	if err != nil {
		t.Fatalf("QuantizeTo failed: %v", err)
	}
	if got.String() != "1.25" {
		t.Fatalf("QuantizeTo = %s, want 1.25", got)
	}

	if _, err := mustParse(t, "1").QuantizeTo(decimal.Decimal{}, decimal.RoundHalfAwayFromZero); !errors.Is(err, decimal.ErrInvalid) {
		t.Fatalf("QuantizeTo zero quantum error = %v, want ErrInvalid", err)
	}
	if _, err := mustParse(t, "1").QuantizeTo(mustParse(t, "-0.05"), decimal.RoundHalfAwayFromZero); !errors.Is(err, decimal.ErrInvalid) {
		t.Fatalf("QuantizeTo negative quantum error = %v, want ErrInvalid", err)
	}
}

func TestRoundedOperations(t *testing.T) {
	got, err := decimal.FromUint64(1).DivToScale(decimal.FromUint64(3), 2, decimal.RoundHalfAwayFromZero)
	if err != nil {
		t.Fatalf("DivToScale failed: %v", err)
	}
	if got.String() != "0.33" {
		t.Fatalf("DivToScale = %s, want 0.33", got)
	}

	got, err = mustParse(t, "1.005").MulToScale(decimal.FromUint64(1), 2, decimal.RoundHalfToEven)
	if err != nil {
		t.Fatalf("MulToScale failed: %v", err)
	}
	if got.String() != "1.00" {
		t.Fatalf("MulToScale = %s, want 1.00", got)
	}

	got, err = mustParse(t, "1.27").MulQuantized(decimal.FromUint64(1), mustParse(t, "0.05"), decimal.RoundHalfAwayFromZero)
	if err != nil {
		t.Fatalf("MulQuantized failed: %v", err)
	}
	if got.String() != "1.25" {
		t.Fatalf("MulQuantized = %s, want 1.25", got)
	}
}

func TestDivToScaleRoundingModes(t *testing.T) {
	tests := []struct {
		mode decimal.RoundingMode
		want string
	}{
		{decimal.RoundHalfAwayFromZero, "0.3"},
		{decimal.RoundHalfToEven, "0.2"},
		{decimal.RoundTowardZero, "0.2"},
		{decimal.RoundTowardNegativeInfinity, "0.2"},
		{decimal.RoundTowardPositiveInfinity, "0.3"},
		{decimal.RoundAwayFromZero, "0.3"},
		{decimal.RoundHalfTowardZero, "0.2"},
	}

	for _, tt := range tests {
		t.Run(roundingModeName(tt.mode), func(t *testing.T) {
			got, err := decimal.FromUint64(1).DivToScale(decimal.FromUint64(4), 1, tt.mode)
			if err != nil {
				t.Fatalf("DivToScale failed: %v", err)
			}
			if got.String() != tt.want {
				t.Fatalf("DivToScale = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestMulAddQuantizedRoundsOnce(t *testing.T) {
	got, err := mustParse(t, "1.005").MulAddToScale(
		decimal.FromUint64(1),
		mustParse(t, "0.005"),
		2,
		decimal.RoundHalfToEven,
	)
	if err != nil {
		t.Fatalf("MulAddToScale failed: %v", err)
	}
	if got.String() != "1.01" {
		t.Fatalf("MulAddToScale = %s, want 1.01", got)
	}
}

func TestRoundRemainsAtMostScale(t *testing.T) {
	got, err := mustParse(t, "1.2300").Round(4, decimal.RoundHalfToEven)
	if err != nil {
		t.Fatalf("Round failed: %v", err)
	}
	if got.String() != "1.2300" {
		t.Fatalf("Round = %s, want 1.2300", got)
	}

	got, err = mustParse(t, "1.2300").Round(2, decimal.RoundHalfToEven)
	if err != nil {
		t.Fatalf("Round failed: %v", err)
	}
	if got.String() != "1.23" {
		t.Fatalf("Round = %s, want 1.23", got)
	}
}

func TestFixedFormatting(t *testing.T) {
	d := mustParse(t, "1.2")
	if got, err := d.StringFixed(2); err != nil || got != "1.20" {
		t.Fatalf("StringFixed = %q err %v, want 1.20 nil", got, err)
	}
	if got := fmt.Sprintf("%6.2f", d); got != "  1.20" {
		t.Fatalf("Sprintf width = %q, want %q", got, "  1.20")
	}
	if got := fmt.Sprintf("%-6.2f", d); got != "1.20  " {
		t.Fatalf("Sprintf left width = %q, want %q", got, "1.20  ")
	}
	if got := fmt.Sprintf("%08.2f", mustParse(t, "-1.2")); got != "-0001.20" {
		t.Fatalf("Sprintf zero-padded negative = %q, want -0001.20", got)
	}
	if got := fmt.Sprintf("%+08.2f", d); got != "+0001.20" {
		t.Fatalf("Sprintf zero-padded positive sign = %q, want +0001.20", got)
	}
	if got, want := fmt.Sprintf("%100.2f", d), strings.Repeat(" ", 96)+"1.20"; got != want {
		t.Fatalf("Sprintf wide padding = %q, want %q", got, want)
	}
	if got, want := fmt.Sprintf("%0100.2f", mustParse(t, "-1.2")), "-"+strings.Repeat("0", 95)+"1.20"; got != want {
		t.Fatalf("Sprintf wide zero-padded negative = %q, want %q", got, want)
	}
	wantMaxFixed := decimal.Max().String() + ".0"
	if got, err := decimal.Max().StringFixed(1); err != nil || got != wantMaxFixed {
		t.Fatalf("Max StringFixed = %q err %v, want %s nil", got, err, wantMaxFixed)
	}
	if got := fmt.Sprintf("%.1f", decimal.Max()); got != wantMaxFixed {
		t.Fatalf("Max Sprintf fixed = %q, want %s", got, wantMaxFixed)
	}
	if got := fmt.Sprintf("%q", d); got != `"1.2"` {
		t.Fatalf("Sprintf quote = %q, want quoted decimal string", got)
	}
	for _, value := range []decimal.Decimal{d, mustParse(t, "-1.2")} {
		for _, format := range []string{"%s", "%8s", "%08s", "%v", "%8v", "%08v", "%.2v", "%#v"} {
			if got, want := fmt.Sprintf(format, value), fmt.Sprintf(format, value.String()); got != want {
				t.Fatalf("Sprintf %q with %s = %q, want %q", format, value, got, want)
			}
		}
	}
	if got := fmt.Sprintf("%.2s", mustParse(t, "123.45")); got != "12" {
		t.Fatalf("Sprintf string precision = %q, want 12", got)
	}
	if got := fmt.Sprintf("%.2f", decimal.Decimal{}); got != "0.00" {
		t.Fatalf("Sprintf zero = %q, want 0.00", got)
	}
	if _, err := mustParse(t, "1.234").StringFixed(2); !errors.Is(err, decimal.ErrInexact) {
		t.Fatalf("StringFixed inexact error = %v, want ErrInexact", err)
	}
	if got, err := mustParse(t, "-1.234").AppendFixed([]byte("x="), 2); !errors.Is(err, decimal.ErrInexact) || string(got) != "x=" {
		t.Fatalf("AppendFixed negative inexact = %q err %v, want original prefix and ErrInexact", got, err)
	}
	if got := fmt.Sprintf("%.29f", d); !strings.Contains(got, "exceeds MaxScale") {
		t.Fatalf("Sprintf invalid precision = %q, want MaxScale marker", got)
	}
}

func TestExactArithmeticMatchesBigRatProperties(t *testing.T) {
	tests := []struct {
		name  string
		check func(testing.TB, decimal.Decimal, decimal.Decimal) bool
	}{
		{"Add", checkExactAddProperty},
		{"Sub", checkExactSubProperty},
		{"Mul", checkExactMulProperty},
		{"Div", checkExactDivProperty},
		{"Remainder", checkExactRemainderProperty},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &quick.Config{
				MaxCount: 2048,
				Rand:     rand.New(rand.NewSource(int64(1000 + i))), //nolint:gosec // Deterministic property data.
			}
			if err := quick.Check(func(a, b quickDecimal) bool {
				return tt.check(t, a.Decimal, b.Decimal)
			}, cfg); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestQuantizedArithmeticMatchesBigRatProperties(t *testing.T) {
	modes := []decimal.RoundingMode{
		decimal.RoundHalfAwayFromZero,
		decimal.RoundHalfToEven,
		decimal.RoundTowardZero,
		decimal.RoundTowardNegativeInfinity,
		decimal.RoundTowardPositiveInfinity,
		decimal.RoundAwayFromZero,
		decimal.RoundHalfTowardZero,
	}
	for i, mode := range modes {
		t.Run(roundingModeName(mode), func(t *testing.T) {
			cfg := &quick.Config{
				MaxCount: 1024,
				Rand:     rand.New(rand.NewSource(int64(2000 + i))), //nolint:gosec // Deterministic property data.
			}
			if err := quick.Check(func(d quickDecimal, scale uint8) bool {
				scale %= decimal.MaxScale + 1
				return checkQuantizeProperty(t, d.Decimal, scale, mode)
			}, cfg); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func mustParse(tb testing.TB, s string) decimal.Decimal {
	tb.Helper()
	d, err := decimal.Parse(s)
	if err != nil {
		tb.Fatalf("Parse(%q): %v", s, err)
	}
	return d
}

func maxMinus(v uint64) decimal.Decimal {
	hi, lo := decimal.MaxMantissa().Words()
	return decimal.MustNew(decimal.NewMantissa(hi, lo-v), 0, false)
}

func checkExactAddProperty(tb testing.TB, a, b decimal.Decimal) bool {
	got, err := a.Add(b)
	want := new(big.Rat).Add(decimalRat(a), decimalRat(b))
	return checkExactDecimalResult(tb, "Add", a, b, got, err, want)
}

func checkExactSubProperty(tb testing.TB, a, b decimal.Decimal) bool {
	got, err := a.Sub(b)
	want := new(big.Rat).Sub(decimalRat(a), decimalRat(b))
	return checkExactDecimalResult(tb, "Sub", a, b, got, err, want)
}

func checkExactMulProperty(tb testing.TB, a, b decimal.Decimal) bool {
	got, err := a.Mul(b)
	want := new(big.Rat).Mul(decimalRat(a), decimalRat(b))
	return checkExactDecimalResult(tb, "Mul", a, b, got, err, want)
}

func checkExactDivProperty(tb testing.TB, a, b decimal.Decimal) bool {
	got, err := a.Div(b)
	if b.IsZero() {
		return checkError(tb, "Div", a, b, err, decimal.ErrDivideByZero)
	}
	want := new(big.Rat).Quo(decimalRat(a), decimalRat(b))
	return checkExactDecimalResult(tb, "Div", a, b, got, err, want)
}

func checkExactRemainderProperty(tb testing.TB, a, b decimal.Decimal) bool {
	got, err := a.Remainder(b)
	if b.IsZero() {
		return checkError(tb, "Remainder", a, b, err, decimal.ErrDivideByZero)
	}
	want, ok := remainderOracle(a, b)
	if !ok {
		return checkError(tb, "Remainder", a, b, err, decimal.ErrOverflow)
	}
	if err != nil {
		tb.Errorf("%s %% %s error = %v, want %s", a, b, err, want)
		return false
	}
	return checkRatEqual(tb, got, want)
}

func checkQuantizeProperty(tb testing.TB, d decimal.Decimal, scale uint8, mode decimal.RoundingMode) bool {
	got, err := d.Quantize(scale, mode)
	want, wantErr := quantizeRat(decimalRat(d), scale, mode)
	if wantErr != nil {
		if !errors.Is(err, wantErr) {
			tb.Errorf("%s.Quantize(%d, %s) error = %v, want %v", d, scale, roundingModeName(mode), err, wantErr)
			return false
		}
		return true
	}
	if err != nil {
		tb.Errorf("%s.Quantize(%d, %s) error = %v, want %s", d, scale, roundingModeName(mode), err, want)
		return false
	}
	if got.IsZero() {
		if want.Sign() != 0 {
			tb.Errorf("%s.Quantize(%d, %s) = zero, want %s", d, scale, roundingModeName(mode), want)
			return false
		}
		return true
	}
	if got.Scale() != scale {
		tb.Errorf("%s.Quantize(%d, %s) scale = %d", d, scale, roundingModeName(mode), got.Scale())
		return false
	}
	return checkRatEqual(tb, got, want)
}

func checkExactDecimalResult(tb testing.TB, op string, a, b, got decimal.Decimal, err error, want *big.Rat) bool {
	wantDecimal, wantErr := exactDecimalOracle(want)
	if wantErr != nil {
		return checkError(tb, op, a, b, err, wantErr)
	}
	if err != nil {
		tb.Errorf("%s %s %s error = %v, want %s", a, op, b, err, wantDecimal)
		return false
	}
	return checkRatEqual(tb, got, want)
}

func checkError(tb testing.TB, op string, a, b decimal.Decimal, err error, want error) bool {
	if !errors.Is(err, want) {
		tb.Errorf("%s %s %s error = %v, want %v", a, op, b, err, want)
		return false
	}
	return true
}

func exactDecimalOracle(r *big.Rat) (decimal.Decimal, error) {
	if r.Sign() == 0 {
		return decimal.Decimal{}, nil
	}
	negative := r.Sign() < 0
	abs := new(big.Rat).Set(r)
	if negative {
		abs.Neg(abs)
	}
	if abs.Cmp(new(big.Rat).SetInt(maxMantissaBig())) > 0 {
		return decimal.Decimal{}, decimal.ErrOverflow
	}
	for scale := 0; scale <= int(decimal.MaxScale); scale++ {
		scaled := new(big.Rat).Mul(abs, new(big.Rat).SetInt(pow10Big(scale)))
		if !scaled.IsInt() {
			continue
		}
		mantissa := scaled.Num()
		if mantissa.Cmp(maxMantissaBig()) <= 0 {
			d, ok := decimalFromBigMantissa(mantissa, uint8(scale), negative)
			if !ok {
				return decimal.Decimal{}, decimal.ErrOverflow
			}
			return d.Normalize(), nil
		}
	}
	return decimal.Decimal{}, decimal.ErrInexact
}

func remainderOracle(a, b decimal.Decimal) (*big.Rat, bool) {
	aMant, aScale, aNeg := a.Parts()
	bMant, bScale, _ := b.Parts()
	scale := max(aScale, bScale)
	aInt := mantissaBig(aMant)
	aInt.Mul(aInt, pow10Big(int(scale-aScale)))
	bInt := mantissaBig(bMant)
	bInt.Mul(bInt, pow10Big(int(scale-bScale)))
	if aInt.Cmp(two192Big()) >= 0 || bInt.Cmp(two192Big()) >= 0 {
		return nil, false
	}
	_, rem := new(big.Int).QuoRem(aInt, bInt, new(big.Int))
	if aNeg {
		rem.Neg(rem)
	}
	return new(big.Rat).SetFrac(rem, pow10Big(int(scale))), true
}

func quantizeRat(r *big.Rat, scale uint8, mode decimal.RoundingMode) (*big.Rat, error) {
	negative := r.Sign() < 0
	abs := new(big.Rat).Set(r)
	if negative {
		abs.Neg(abs)
	}
	quantum := new(big.Int).SetInt64(1)
	scaled := new(big.Rat).Mul(abs, new(big.Rat).SetInt(pow10Big(int(scale))))
	q, rem := new(big.Int).QuoRem(scaled.Num(), scaled.Denom(), new(big.Int))
	if shouldRoundUpRat(mode, negative, q, rem, scaled.Denom()) {
		q.Add(q, quantum)
	}
	if q.Cmp(maxMantissaBig()) > 0 {
		return nil, decimal.ErrOverflow
	}
	if negative {
		q.Neg(q)
	}
	return new(big.Rat).SetFrac(q, pow10Big(int(scale))), nil
}

func shouldRoundUpRat(mode decimal.RoundingMode, negative bool, quotient, remainder, divisor *big.Int) bool {
	if remainder.Sign() == 0 {
		return false
	}
	switch mode {
	case decimal.RoundHalfAwayFromZero:
		return twice(remainder).Cmp(divisor) >= 0
	case decimal.RoundHalfToEven:
		cmp := twice(remainder).Cmp(divisor)
		return cmp > 0 || cmp == 0 && quotient.Bit(0) == 1
	case decimal.RoundTowardZero:
		return false
	case decimal.RoundTowardNegativeInfinity:
		return negative
	case decimal.RoundTowardPositiveInfinity:
		return !negative
	case decimal.RoundAwayFromZero:
		return true
	case decimal.RoundHalfTowardZero:
		return twice(remainder).Cmp(divisor) > 0
	default:
		panic("decimal test: invalid rounding mode")
	}
}

func checkRatEqual(tb testing.TB, got decimal.Decimal, want *big.Rat) bool {
	if decimalRat(got).Cmp(want) != 0 {
		tb.Errorf("got %s (%s), want %s", got, decimalRat(got), want)
		return false
	}
	return true
}

func decimalRat(d decimal.Decimal) *big.Rat {
	mantissa, scale, negative := d.Parts()
	r := new(big.Rat).SetInt(mantissaBig(mantissa))
	if scale > 0 {
		r.Quo(r, new(big.Rat).SetInt(pow10Big(int(scale))))
	}
	if negative {
		r.Neg(r)
	}
	return r
}

func decimalFromBigMantissa(n *big.Int, scale uint8, negative bool) (decimal.Decimal, bool) {
	if n.Sign() == 0 {
		return decimal.Decimal{}, true
	}
	if n.Cmp(maxMantissaBig()) > 0 {
		return decimal.Decimal{}, false
	}
	loMask := new(big.Int).SetUint64(^uint64(0))
	lo := new(big.Int).And(new(big.Int).Set(n), loMask).Uint64()
	hi := new(big.Int).Rsh(new(big.Int).Set(n), 64).Uint64()
	if hi > uint64(^uint32(0)) {
		return decimal.Decimal{}, false
	}
	d, err := decimal.New(decimal.NewMantissa(uint32(hi), lo), scale, negative)
	return d, err == nil
}

func randomDecimal(rng *rand.Rand) decimal.Decimal {
	return decimal.MustNew(decimal.NewMantissa(rng.Uint32(), rng.Uint64()), randomScale(rng), rng.Intn(2) == 0)
}

func randomScale(rng *rand.Rand) uint8 {
	return uint8(rng.Intn(int(decimal.MaxScale) + 1))
}

func roundingModeName(mode decimal.RoundingMode) string {
	switch mode {
	case decimal.RoundHalfAwayFromZero:
		return "RoundHalfAwayFromZero"
	case decimal.RoundHalfToEven:
		return "RoundHalfToEven"
	case decimal.RoundTowardZero:
		return "RoundTowardZero"
	case decimal.RoundTowardNegativeInfinity:
		return "RoundTowardNegativeInfinity"
	case decimal.RoundTowardPositiveInfinity:
		return "RoundTowardPositiveInfinity"
	case decimal.RoundAwayFromZero:
		return "RoundAwayFromZero"
	case decimal.RoundHalfTowardZero:
		return "RoundHalfTowardZero"
	default:
		return fmt.Sprintf("RoundingMode(%d)", mode)
	}
}

func mantissaBig(m decimal.Mantissa) *big.Int {
	hi, lo := m.Words()
	out := new(big.Int).SetUint64(uint64(hi))
	out.Lsh(out, 64)
	return out.Add(out, new(big.Int).SetUint64(lo))
}

func maxMantissaBig() *big.Int {
	return mantissaBig(decimal.MaxMantissa())
}

func pow10Big(scale int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
}

func two192Big() *big.Int {
	return new(big.Int).Lsh(big.NewInt(1), 192)
}

func twice(n *big.Int) *big.Int {
	return new(big.Int).Lsh(n, 1)
}
