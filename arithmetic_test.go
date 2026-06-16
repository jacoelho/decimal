package decimal_test

import (
	"math/big"
	"math/rand"
	"strings"
	"testing"

	"github.com/jacoelho/decimal"
)

func TestAddSubExamplesWithBigRatOracle(t *testing.T) {
	tests := []struct {
		a, b string
	}{
		{"1.23", "0.0001"},
		{"999999.99", "-0.01"},
		{"-12.50", "-7.25"},
		{"123456789.123456", "987654321.000001"},
	}

	for _, tt := range tests {
		t.Run(tt.a+"+"+tt.b, func(t *testing.T) {
			a := mustParse(t, tt.a)
			b := mustParse(t, tt.b)
			got, ok := a.Add(b)
			if !ok {
				t.Fatalf("Add overflow")
			}
			want := new(big.Rat).Add(decimalRat(a), decimalRat(b))
			assertRatEqual(t, got, want)

			back, ok := got.Sub(b)
			if !ok {
				t.Fatalf("Sub overflow")
			}
			if !back.Equal(a) {
				t.Fatalf("(%s + %s) - %s = %s, want %s", a, b, b, back, a)
			}
		})
	}
}

func TestAddRoundsByReducingScaleWhenMantissaOverflows(t *testing.T) {
	a := decimal.MustNew(decimal.MaxMantissa(), 1, false)
	got, ok := a.Add(a)
	if !ok {
		t.Fatalf("Add overflow")
	}
	if got.Scale() != 0 {
		t.Fatalf("scale = %d, want 0", got.Scale())
	}

	want := new(big.Rat).Add(decimalRat(a), decimalRat(a))
	gotRat := decimalRat(got)
	diff := new(big.Rat).Sub(gotRat, want)
	half := new(big.Rat).SetFrac(big.NewInt(1), big.NewInt(2))
	if diff.Abs(diff).Cmp(half) > 0 {
		t.Fatalf("rounded result too far: got %s want %s diff %s", got, want, diff)
	}
}

func TestAddOverflowsAtScaleZero(t *testing.T) {
	_, ok := decimal.Max().Add(decimal.Max())
	if ok {
		t.Fatalf("Max + Max succeeded, want overflow")
	}
}

func TestMulExamplesWithBigRatOracle(t *testing.T) {
	tests := []struct {
		a, b string
		want string
	}{
		{"1.23", "0.0001", "0.000123"},
		{"-12.5", "2.00", "-25.000"},
		{"123456", "1000", "123456000"},
	}

	for _, tt := range tests {
		t.Run(tt.a+"*"+tt.b, func(t *testing.T) {
			a := mustParse(t, tt.a)
			b := mustParse(t, tt.b)
			got, ok := a.Mul(b)
			if !ok {
				t.Fatalf("Mul overflow")
			}
			if got.String() != tt.want {
				t.Fatalf("Mul = %s, want %s", got, tt.want)
			}
			want := new(big.Rat).Mul(decimalRat(a), decimalRat(b))
			assertRatEqual(t, got, want)
		})
	}
}

func TestMulByOnePreservesMaxScaleMantissa(t *testing.T) {
	a := decimal.MustNew(decimal.MaxMantissa(), decimal.MaxScale, false)
	one := decimal.FromUint64(1)
	got, ok := a.Mul(one)
	if !ok {
		t.Fatalf("Mul overflow")
	}
	if !got.Equal(a) {
		t.Fatalf("%s * 1 = %s", a, got)
	}
}

func TestDivExamples(t *testing.T) {
	tests := []struct {
		a, b string
		want string
	}{
		{"1.23", "0.0001", "12300"},
		{"1.23", "2", "0.615"},
		{"1", "8", "0.125"},
		{"1", "3", "0.3333333333333333333333333333"},
		{"-10", "4", "-2.5"},
	}

	for _, tt := range tests {
		t.Run(tt.a+"/"+tt.b, func(t *testing.T) {
			a := mustParse(t, tt.a)
			b := mustParse(t, tt.b)
			got, ok := a.Div(b)
			if !ok {
				t.Fatalf("Div failed")
			}
			if got.String() != tt.want {
				t.Fatalf("Div = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestDivByZeroFails(t *testing.T) {
	_, ok := decimal.FromUint64(1).Div(decimal.Decimal{})
	if ok {
		t.Fatalf("division by zero succeeded")
	}
}

func TestDivByOneNormalizesExactResult(t *testing.T) {
	d := decimal.MustNew(decimal.MantissaFromUint64(1234500), 4, false)
	got, ok := d.Div(decimal.FromUint64(1))
	if !ok {
		t.Fatalf("Div by one failed")
	}
	if got.String() != "123.45" {
		t.Fatalf("%s / 1 = %s, want 123.45", d, got)
	}

	got, ok = d.Div(decimal.FromInt64(-1))
	if !ok {
		t.Fatalf("Div by negative one failed")
	}
	if got.String() != "-123.45" {
		t.Fatalf("%s / -1 = %s, want -123.45", d, got)
	}
}

func TestDivByPowerOfTenDivisorUsesExactScaleShift(t *testing.T) {
	divisor := decimal.MustNew(decimal.MantissaFromUint64(1), decimal.MaxScale, false)
	got, ok := decimal.FromUint64(1).Div(divisor)
	if !ok {
		t.Fatalf("Div by 1e-28 failed")
	}
	if got.String() != "10000000000000000000000000000" {
		t.Fatalf("1 / %s = %s, want 1e28", divisor, got)
	}

	divisor = decimal.MustNew(decimal.MantissaFromUint64(10), decimal.MaxScale, false)
	got, ok = decimal.FromUint64(1).Div(divisor)
	if !ok {
		t.Fatalf("Div by non-canonical 1e-27 failed")
	}
	if got.String() != "1000000000000000000000000000" {
		t.Fatalf("1 / %s = %s, want 1e27", divisor, got)
	}

	if _, ok := decimal.Max().Div(decimal.MustNew(decimal.MantissaFromUint64(1), 1, false)); ok {
		t.Fatalf("Max / 0.1 succeeded, want overflow")
	}
}

func TestRemainderExamplesMatchDotNetScale(t *testing.T) {
	tests := []struct {
		a, b string
		want string
	}{
		{"16.8", "4.1", "0.4"},
		{"5.00", "2.00", "1.00"},
		{"5.0", "2", "1.0"},
		{"-5.00", "2.00", "-1.00"},
		{"1.234", "0.01", "0.004"},
	}

	for _, tt := range tests {
		t.Run(tt.a+"%"+tt.b, func(t *testing.T) {
			a := mustParse(t, tt.a)
			b := mustParse(t, tt.b)
			got, ok := a.Remainder(b)
			if !ok {
				t.Fatalf("Remainder failed")
			}
			if got.String() != tt.want {
				t.Fatalf("Remainder = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRemainderByZeroFails(t *testing.T) {
	_, ok := decimal.FromUint64(1).Remainder(decimal.Decimal{})
	if ok {
		t.Fatalf("remainder by zero succeeded")
	}
}

func TestRemainderReturnsDividendWhenScaledDivisorOverflowsMantissa(t *testing.T) {
	dividend := decimal.MustNew(decimal.MaxMantissa(), decimal.MaxScale, false)
	got, ok := dividend.Remainder(decimal.Max())
	if !ok {
		t.Fatalf("Remainder failed")
	}
	if !sameBits(got, dividend) {
		t.Fatalf("%s %% Max = %s, want original representation", dividend, got)
	}
}

func TestDivSkipsOverflowedScaleAndFindsRepresentableQuotient(t *testing.T) {
	denom := decimal.MustNew(decimal.MaxMantissa(), decimal.MaxScale, false)
	got, ok := decimal.Max().Div(denom)
	if !ok {
		t.Fatalf("Div failed")
	}
	if got.String() != "10000000000000000000000000000" {
		t.Fatalf("Div = %s, want 10000000000000000000000000000", got)
	}
}

func TestRoundToRoundsHalfAwayFromZero(t *testing.T) {
	d := mustParse(t, "12.345678")
	if got := d.RoundTo(2).String(); got != "12.35" {
		t.Fatalf("RoundTo(2) = %s, want 12.35", got)
	}
	if got := d.RoundTo(decimal.MaxScale + 1); !sameBits(got, d) {
		t.Fatalf("RoundTo(MaxScale+1) = %s, want original representation", got)
	}
}

func TestRoundModes(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		scale uint8
		mode  decimal.RoundingMode
		want  string
	}{
		{"half away positive", "1.235", 2, decimal.RoundHalfAwayFromZero, "1.24"},
		{"half away negative", "-1.235", 2, decimal.RoundHalfAwayFromZero, "-1.24"},
		{"half even down", "2.5", 0, decimal.RoundHalfToEven, "2"},
		{"half even up", "3.5", 0, decimal.RoundHalfToEven, "4"},
		{"half even scaled down", "1.2450", 2, decimal.RoundHalfToEven, "1.24"},
		{"half even scaled up", "1.2550", 2, decimal.RoundHalfToEven, "1.26"},
		{"toward zero positive", "1.239", 2, decimal.RoundTowardZero, "1.23"},
		{"toward zero negative", "-1.239", 2, decimal.RoundTowardZero, "-1.23"},
		{"toward positive infinity positive", "1.231", 2, decimal.RoundTowardPositiveInfinity, "1.24"},
		{"toward positive infinity negative", "-1.239", 2, decimal.RoundTowardPositiveInfinity, "-1.23"},
		{"toward negative infinity positive", "1.231", 2, decimal.RoundTowardNegativeInfinity, "1.23"},
		{"toward negative infinity negative", "-1.231", 2, decimal.RoundTowardNegativeInfinity, "-1.24"},
		{"preserves scale when not reducing", "1.2300", 4, decimal.RoundHalfToEven, "1.2300"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := mustParse(t, tt.in).Round(tt.scale, tt.mode)
			if !ok {
				t.Fatalf("Round failed")
			}
			if got.String() != tt.want {
				t.Fatalf("Round = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRoundRejectsInvalidScaleOrMode(t *testing.T) {
	d := mustParse(t, "1.23")
	if _, ok := d.Round(decimal.MaxScale+1, decimal.RoundHalfToEven); ok {
		t.Fatalf("Round with invalid scale succeeded")
	}
	if _, ok := d.Round(1, decimal.RoundingMode(255)); ok {
		t.Fatalf("Round with invalid mode succeeded")
	}
}

func TestRescaleIncreasesScaleWithTrailingZeros(t *testing.T) {
	rescaled, ok := decimal.FromUint64(12).Rescale(2)
	if !ok {
		t.Fatalf("Rescale failed")
	}
	if rescaled.String() != "12.00" {
		t.Fatalf("Rescale = %s, want 12.00", rescaled)
	}
}

func TestArithmeticEdgeCasesWithBigRatOracle(t *testing.T) {
	values := []decimal.Decimal{
		decimal.Decimal{},
		decimal.FromInt64(1),
		decimal.FromInt64(-1),
		decimal.MustNew(decimal.MantissaFromUint64(120), 2, false),
		decimal.MustNew(decimal.MantissaFromUint64(340), 2, true),
		decimal.MustNew(decimal.MaxMantissa(), decimal.MaxScale, false),
		decimal.MustNew(decimal.MaxMantissa(), decimal.MaxScale, true),
		decimal.Max(),
		decimal.Min(),
	}

	for _, a := range values {
		for _, b := range values {
			assertBinaryArithmetic(t, a, b)
		}
	}
}

func TestRandomArithmeticWithBigRatOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // Deterministic test data, not cryptographic randomness.
	for range 2048 {
		assertBinaryArithmetic(t, randomDecimal(rng), randomDecimal(rng))
	}
}

func TestAddRoundingBoundaries(t *testing.T) {
	tests := []struct {
		name string
		add  decimal.Decimal
		want decimal.Decimal
		ok   bool
	}{
		{
			name: "below half keeps max",
			add:  decimal.MustNew(decimal.MantissaFromUint64(4), 1, false),
			want: decimal.Max(),
			ok:   true,
		},
		{
			name: "half away from zero overflows",
			add:  decimal.MustNew(decimal.MantissaFromUint64(5), 1, false),
			ok:   false,
		},
		{
			name: "above half overflows",
			add:  decimal.MustNew(decimal.MantissaFromUint64(6), 1, false),
			ok:   false,
		},
		{
			name: "negative half rounds after subtraction",
			add:  decimal.MustNew(decimal.MantissaFromUint64(5), 1, true),
			want: decimal.Max(),
			ok:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := decimal.Max().Add(tt.add)
			if ok != tt.ok {
				t.Fatalf("Max.Add(%s) ok = %v, want %v", tt.add, ok, tt.ok)
			}
			if ok && !got.Equal(tt.want) {
				t.Fatalf("Max.Add(%s) = %s, want %s", tt.add, got, tt.want)
			}
		})
	}
}

func FuzzDecimalArithmeticWithBigRatOracle(f *testing.F) {
	maxHi, maxLo := decimal.MaxMantissa().Words()
	for _, seed := range []struct {
		aHi    uint32
		aLo    uint64
		aScale uint8
		aNeg   bool
		bHi    uint32
		bLo    uint64
		bScale uint8
		bNeg   bool
	}{
		{0, 123, 2, false, 0, 45, 1, true},
		{0, 1, decimal.MaxScale, false, 0, 3, 0, false},
		{maxHi, maxLo, 0, false, 0, 5, 1, true},
	} {
		f.Add(seed.aHi, seed.aLo, seed.aScale, seed.aNeg, seed.bHi, seed.bLo, seed.bScale, seed.bNeg)
	}

	f.Fuzz(func(t *testing.T, aHi uint32, aLo uint64, aScale uint8, aNeg bool, bHi uint32, bLo uint64, bScale uint8, bNeg bool) {
		a := decimal.MustNew(decimal.NewMantissa(aHi, aLo), aScale%(decimal.MaxScale+1), aNeg)
		b := decimal.MustNew(decimal.NewMantissa(bHi, bLo), bScale%(decimal.MaxScale+1), bNeg)
		assertBinaryArithmetic(t, a, b)
	})
}

func mustParse(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return d
}

func assertBinaryArithmetic(t *testing.T, a, b decimal.Decimal) {
	t.Helper()

	aRat := decimalRat(a)
	bRat := decimalRat(b)
	aScale := a.Scale()
	bScale := b.Scale()

	want, wantOK := roundedAtScale(new(big.Rat).Add(aRat, bRat), int(max(aScale, bScale)))
	got, ok := a.Add(b)
	assertDecimalResult(t, a, "+", b, got, ok, want, wantOK)

	want, wantOK = roundedAtScale(new(big.Rat).Sub(aRat, bRat), int(max(aScale, bScale)))
	got, ok = a.Sub(b)
	assertDecimalResult(t, a, "-", b, got, ok, want, wantOK)

	want, wantOK = roundedAtScale(new(big.Rat).Mul(aRat, bRat), int(aScale)+int(bScale))
	got, ok = a.Mul(b)
	assertDecimalResult(t, a, "*", b, got, ok, want, wantOK)

	if b.Sign() == 0 {
		want, wantOK = "", false
	} else {
		want, wantOK = roundedDiv(new(big.Rat).Quo(aRat, bRat))
	}
	got, ok = a.Div(b)
	assertDecimalResult(t, a, "/", b, got, ok, want, wantOK)

	if b.Sign() == 0 {
		want, wantOK = "", false
	} else {
		want, wantOK = remainderAtCommonScale(a, b)
	}
	got, ok = a.Remainder(b)
	assertDecimalResult(t, a, "%", b, got, ok, want, wantOK)
}

func randomDecimal(rng *rand.Rand) decimal.Decimal {
	mantissa := decimal.NewMantissa(rng.Uint32(), rng.Uint64())
	scale := randomScale(rng)
	return decimal.MustNew(mantissa, scale, rng.Intn(2) == 0)
}

func randomScale(rng *rand.Rand) uint8 {
	scale := rng.Intn(int(decimal.MaxScale) + 1)
	if scale < 0 || scale > int(decimal.MaxScale) {
		panic("decimal test: random scale out of range")
	}
	return uint8(scale)
}

func assertDecimalResult(t *testing.T, a decimal.Decimal, op string, b decimal.Decimal, got decimal.Decimal, ok bool, want string, wantOK bool) {
	t.Helper()
	if ok != wantOK {
		t.Fatalf("%s %s %s ok = %v, want %v", a, op, b, ok, wantOK)
	}
	if ok && got.String() != want {
		t.Fatalf("%s %s %s = %s, want %s", a, op, b, got, want)
	}
}

func roundedAtScale(r *big.Rat, initialScale int) (string, bool) {
	negative := r.Sign() < 0
	if negative {
		r = new(big.Rat).Neg(r)
	}

	for scale := min(initialScale, int(decimal.MaxScale)); scale >= 0; scale-- {
		q, _ := roundRat(r, scale)
		if q.Cmp(mantissaBig(decimal.MaxMantissa())) > 0 {
			continue
		}
		return formatOracle(q, scale, negative), true
	}
	return "", false
}

func roundedDiv(r *big.Rat) (string, bool) {
	negative := r.Sign() < 0
	if negative {
		r = new(big.Rat).Neg(r)
	}

	for scale := int(decimal.MaxScale); scale >= 0; scale-- {
		q, rem := roundRat(r, scale)
		if q.Cmp(mantissaBig(decimal.MaxMantissa())) > 0 {
			continue
		}
		if rem.Sign() == 0 {
			ten := big.NewInt(10)
			for scale > 0 {
				next, trailing := new(big.Int).QuoRem(q, ten, new(big.Int))
				if trailing.Sign() != 0 {
					break
				}
				q = next
				scale--
			}
		}
		return formatOracle(q, scale, negative), true
	}
	return "", false
}

func remainderAtCommonScale(a, b decimal.Decimal) (string, bool) {
	aMant, aScale, aNeg := a.Parts()
	bMant, bScale, _ := b.Parts()
	if bMant == (decimal.Mantissa{}) {
		return "", false
	}

	scale := int(max(aScale, bScale))
	aInt := mantissaBig(aMant)
	aInt.Mul(aInt, pow10Big(scale-int(aScale)))
	bInt := mantissaBig(bMant)
	bInt.Mul(bInt, pow10Big(scale-int(bScale)))
	_, rem := new(big.Int).QuoRem(aInt, bInt, new(big.Int))
	return formatOracle(rem, scale, aNeg), true
}

func roundRat(r *big.Rat, scale int) (*big.Int, *big.Int) {
	scaled := new(big.Int).Mul(r.Num(), pow10Big(scale))
	q, rem := new(big.Int).QuoRem(scaled, r.Denom(), new(big.Int))
	if new(big.Int).Lsh(rem, 1).Cmp(r.Denom()) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	return q, rem
}

func formatOracle(q *big.Int, scale int, negative bool) string {
	if q.Sign() == 0 {
		return "0"
	}
	s := formatScaledInt(q.String(), scale)
	if negative {
		return "-" + s
	}
	return s
}

func formatScaledInt(digits string, scale int) string {
	if scale == 0 {
		return digits
	}
	if len(digits) <= scale {
		return "0." + strings.Repeat("0", scale-len(digits)) + digits
	}
	i := len(digits) - scale
	return digits[:i] + "." + digits[i:]
}

func assertRatEqual(t *testing.T, got decimal.Decimal, want *big.Rat) {
	t.Helper()
	if decimalRat(got).Cmp(want) != 0 {
		t.Fatalf("got %s (%s), want %s", got, decimalRat(got), want)
	}
}

func decimalRat(d decimal.Decimal) *big.Rat {
	m, scale, neg := d.Parts()
	r := new(big.Rat).SetInt(mantissaBig(m))
	if scale > 0 {
		r.Quo(r, new(big.Rat).SetInt(pow10Big(int(scale))))
	}
	if neg {
		r.Neg(r)
	}
	return r
}

func mantissaBig(u decimal.Mantissa) *big.Int {
	hiWord, loWord := u.Words()
	hi := new(big.Int).SetUint64(uint64(hiWord))
	hi.Lsh(hi, 64)
	return hi.Add(hi, new(big.Int).SetUint64(loWord))
}

func pow10Big(scale int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
}
