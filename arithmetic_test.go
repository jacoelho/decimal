package decimal_test

import (
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

func TestArithmeticOperationEdgeCases(t *testing.T) {
	tests := []struct {
		op   string
		edge string
		a    string
		b    string
		want string
		ok   bool
	}{
		{op: "Add", edge: "left zero returns right operand representation", a: "0", b: "-1.2300", want: "-1.2300", ok: true},
		{op: "Add", edge: "right zero preserves left operand representation", a: "1.2300", b: "0", want: "1.2300", ok: true},
		{op: "Add", edge: "opposite signs cancel to canonical zero", a: "1.2300", b: "-1.2300", want: "0", ok: true},
		{op: "Add", edge: "aligns unequal scales before adding", a: "1.2", b: "0.03", want: "1.23", ok: true},
		{op: "Add", edge: "same sign negative operands", a: "-1.20", b: "-0.03", want: "-1.23", ok: true},
		{op: "Add", edge: "integer overflow fails", a: decimal.Max().String(), b: decimal.Max().String(), ok: false},

		{op: "Sub", edge: "subtracting zero preserves left operand representation", a: "1.2300", b: "0", want: "1.2300", ok: true},
		{op: "Sub", edge: "zero minus nonzero negates with operand scale", a: "0", b: "1.2300", want: "-1.2300", ok: true},
		{op: "Sub", edge: "equal operands cancel to canonical zero", a: "1.2300", b: "1.2300", want: "0", ok: true},
		{op: "Sub", edge: "borrows across aligned fractional scales", a: "1.00", b: "0.001", want: "0.999", ok: true},
		{op: "Sub", edge: "negative minus positive keeps negative sign", a: "-1.2", b: "0.3", want: "-1.5", ok: true},
		{op: "Sub", edge: "rounding below Min fails", a: decimal.Min().String(), b: "0.5", ok: false},

		{op: "Mul", edge: "zero operand returns canonical zero", a: "123.45", b: "0", want: "0", ok: true},
		{op: "Mul", edge: "two negatives produce positive product", a: "-1.20", b: "-0.5", want: "0.600", ok: true},
		{op: "Mul", edge: "result scale is sum of operand scales when exact", a: "1.20", b: "0.030", want: "0.03600", ok: true},
		{op: "Mul", edge: "half quantum rounds away from zero", a: "0.5", b: "0.0000000000000000000000000001", want: "0.0000000000000000000000000001", ok: true},
		{op: "Mul", edge: "tiny product rounds to canonical zero", a: "0.0000000000000000000000000001", b: "0.0000000000000000000000000001", want: "0", ok: true},
		{op: "Mul", edge: "integer overflow fails", a: decimal.Max().String(), b: "2", ok: false},

		{op: "Div", edge: "division by zero fails", a: "1", b: "0", ok: false},
		{op: "Div", edge: "zero dividend returns canonical zero", a: "0", b: "-1.23", want: "0", ok: true},
		{op: "Div", edge: "exact quotient is normalized", a: "1.00", b: "0.25", want: "4", ok: true},
		{op: "Div", edge: "negative signs cancel", a: "-10", b: "-4", want: "2.5", ok: true},
		{op: "Div", edge: "repeating quotient rounds at MaxScale", a: "1", b: "6", want: "0.1666666666666666666666666667", ok: true},
		{op: "Div", edge: "quotient overflow fails", a: decimal.Max().String(), b: "0.1", ok: false},

		{op: "Remainder", edge: "remainder by zero fails", a: "1", b: "0", ok: false},
		{op: "Remainder", edge: "zero dividend returns canonical zero", a: "0", b: "-1.23", want: "0", ok: true},
		{op: "Remainder", edge: "sign follows dividend", a: "-5.5", b: "2", want: "-1.5", ok: true},
		{op: "Remainder", edge: "common scale is preserved for nonzero remainder", a: "5.00", b: "2", want: "1.00", ok: true},
		{op: "Remainder", edge: "divisor larger than dividend returns dividend", a: "1.23", b: "4", want: "1.23", ok: true},
		{op: "Remainder", edge: "exact multiple returns canonical zero", a: "4.00", b: "2.00", want: "0", ok: true},
	}

	for _, tt := range tests {
		t.Run(tt.op+"/"+tt.edge, func(t *testing.T) {
			a := mustParse(t, tt.a)
			b := mustParse(t, tt.b)
			got, ok := runArithmeticOperation(tt.op, a, b)
			if ok != tt.ok {
				t.Fatalf("%s %s %s ok = %v, want %v", a, tt.op, b, ok, tt.ok)
			}
			if ok && got.String() != tt.want {
				t.Fatalf("%s %s %s = %s, want %s", a, tt.op, b, got, tt.want)
			}
		})
	}
}

func TestRandomArithmeticWithBigRatOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // Deterministic test data, not cryptographic randomness.
	for range 2048 {
		assertBinaryArithmetic(t, randomDecimal(rng), randomDecimal(rng))
	}
}

func TestArithmeticOperationsMatchBigRatProperties(t *testing.T) {
	tests := []struct {
		name  string
		check func(testing.TB, decimal.Decimal, decimal.Decimal) bool
	}{
		{"Add", checkAddProperty},
		{"Sub", checkSubProperty},
		{"Mul", checkMulProperty},
		{"Div", checkDivProperty},
		{"Remainder", checkRemainderProperty},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &quick.Config{
				MaxCount: 2048,
				Rand:     rand.New(rand.NewSource(int64(100 + i))), //nolint:gosec // Deterministic property data.
			}
			if err := quick.Check(func(a, b quickDecimal) bool {
				return tt.check(t, a.Decimal, b.Decimal)
			}, cfg); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRescaleMatchesBigRatProperties(t *testing.T) {
	cfg := &quick.Config{
		MaxCount: 2048,
		Rand:     rand.New(rand.NewSource(200)), //nolint:gosec // Deterministic property data.
	}
	if err := quick.Check(func(d quickDecimal, scale uint8) bool {
		return checkRescaleProperty(t, d.Decimal, scale)
	}, cfg); err != nil {
		t.Fatal(err)
	}
}

func TestRoundMatchesBigIntProperties(t *testing.T) {
	modes := []decimal.RoundingMode{
		decimal.RoundHalfAwayFromZero,
		decimal.RoundHalfToEven,
		decimal.RoundTowardZero,
		decimal.RoundTowardNegativeInfinity,
		decimal.RoundTowardPositiveInfinity,
	}
	for i, mode := range modes {
		t.Run(roundingModeName(mode), func(t *testing.T) {
			cfg := &quick.Config{
				MaxCount: 2048,
				Rand:     rand.New(rand.NewSource(int64(300 + i))), //nolint:gosec // Deterministic property data.
			}
			if err := quick.Check(func(d quickDecimal, scale uint8) bool {
				return checkRoundProperty(t, d.Decimal, scale, mode)
			}, cfg); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRoundToMatchesRoundHalfAwayProperties(t *testing.T) {
	cfg := &quick.Config{
		MaxCount: 2048,
		Rand:     rand.New(rand.NewSource(400)), //nolint:gosec // Deterministic property data.
	}
	if err := quick.Check(func(d quickDecimal, scale uint8) bool {
		return checkRoundToProperty(t, d.Decimal, scale)
	}, cfg); err != nil {
		t.Fatal(err)
	}
}

func TestIntegralArithmeticOperationsMatchBigIntProperties(t *testing.T) {
	cfg := &quick.Config{
		MaxCount: 2048,
		Rand:     rand.New(rand.NewSource(500)), //nolint:gosec // Deterministic property data.
	}
	if err := quick.Check(func(d quickDecimal) bool {
		return checkIntegralProperty(t, d.Decimal)
	}, cfg); err != nil {
		t.Fatal(err)
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

func runArithmeticOperation(op string, a, b decimal.Decimal) (decimal.Decimal, bool) {
	switch op {
	case "Add":
		return a.Add(b)
	case "Sub":
		return a.Sub(b)
	case "Mul":
		return a.Mul(b)
	case "Div":
		return a.Div(b)
	case "Remainder":
		return a.Remainder(b)
	default:
		panic("unknown arithmetic operation: " + op)
	}
}

func checkAddProperty(tb testing.TB, a, b decimal.Decimal) bool {
	tb.Helper()
	want, wantOK := roundedAtScale(new(big.Rat).Add(decimalRat(a), decimalRat(b)), int(max(a.Scale(), b.Scale())))
	got, ok := a.Add(b)
	return checkDecimalResult(tb, a, "+", b, got, ok, want, wantOK)
}

func checkSubProperty(tb testing.TB, a, b decimal.Decimal) bool {
	tb.Helper()
	want, wantOK := roundedAtScale(new(big.Rat).Sub(decimalRat(a), decimalRat(b)), int(max(a.Scale(), b.Scale())))
	got, ok := a.Sub(b)
	return checkDecimalResult(tb, a, "-", b, got, ok, want, wantOK)
}

func checkMulProperty(tb testing.TB, a, b decimal.Decimal) bool {
	tb.Helper()
	want, wantOK := roundedAtScale(new(big.Rat).Mul(decimalRat(a), decimalRat(b)), int(a.Scale())+int(b.Scale()))
	got, ok := a.Mul(b)
	return checkDecimalResult(tb, a, "*", b, got, ok, want, wantOK)
}

func checkDivProperty(tb testing.TB, a, b decimal.Decimal) bool {
	tb.Helper()
	want, wantOK := "", false
	if b.Sign() != 0 {
		want, wantOK = roundedDiv(new(big.Rat).Quo(decimalRat(a), decimalRat(b)))
	}
	got, ok := a.Div(b)
	return checkDecimalResult(tb, a, "/", b, got, ok, want, wantOK)
}

func checkRemainderProperty(tb testing.TB, a, b decimal.Decimal) bool {
	tb.Helper()
	want, wantOK := "", false
	if b.Sign() != 0 {
		want, wantOK = remainderAtCommonScale(a, b)
	}
	got, ok := a.Remainder(b)
	return checkDecimalResult(tb, a, "%", b, got, ok, want, wantOK)
}

func checkRescaleProperty(tb testing.TB, d decimal.Decimal, scale uint8) bool {
	tb.Helper()
	want, wantOK := rescaleOracle(d, scale)
	got, ok := d.Rescale(scale)
	if ok != wantOK {
		tb.Logf("%s.Rescale(%d) ok = %v, want %v", d, scale, ok, wantOK)
		return false
	}
	if ok && got.String() != want {
		tb.Logf("%s.Rescale(%d) = %s, want %s", d, scale, got, want)
		return false
	}
	return true
}

func checkRoundProperty(tb testing.TB, d decimal.Decimal, scale uint8, mode decimal.RoundingMode) bool {
	tb.Helper()
	want, wantOK := roundOracle(d, scale, mode)
	got, ok := d.Round(scale, mode)
	if ok != wantOK {
		tb.Logf("%s.Round(%d, %s) ok = %v, want %v", d, scale, roundingModeName(mode), ok, wantOK)
		return false
	}
	if ok && got.String() != want {
		tb.Logf("%s.Round(%d, %s) = %s, want %s", d, scale, roundingModeName(mode), got, want)
		return false
	}
	return true
}

func checkRoundToProperty(tb testing.TB, d decimal.Decimal, scale uint8) bool {
	tb.Helper()
	got := d.RoundTo(scale)
	if scale >= d.Scale() {
		if !sameBits(got, d) {
			tb.Logf("%s.RoundTo(%d) = %s with different bits, want original representation", d, scale, got)
			return false
		}
		return true
	}

	want, wantOK := roundOracle(d, scale, decimal.RoundHalfAwayFromZero)
	if !wantOK {
		tb.Logf("%s.RoundTo(%d) oracle overflowed", d, scale)
		return false
	}
	if got.String() != want {
		tb.Logf("%s.RoundTo(%d) = %s, want %s", d, scale, got, want)
		return false
	}
	return true
}

func checkIntegralProperty(tb testing.TB, d decimal.Decimal) bool {
	tb.Helper()
	tests := []struct {
		name string
		got  decimal.Decimal
		want string
	}{
		{"Truncate", d.Truncate(), integralOracle(d, "truncate")},
		{"Floor", d.Floor(), integralOracle(d, "floor")},
		{"Ceiling", d.Ceiling(), integralOracle(d, "ceiling")},
	}
	for _, tt := range tests {
		if tt.got.String() != tt.want {
			tb.Logf("%s.%s() = %s, want %s", d, tt.name, tt.got, tt.want)
			return false
		}
	}
	return true
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
	default:
		return "invalid"
	}
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

func checkDecimalResult(tb testing.TB, a decimal.Decimal, op string, b decimal.Decimal, got decimal.Decimal, ok bool, want string, wantOK bool) bool {
	tb.Helper()
	if ok != wantOK {
		tb.Logf("%s %s %s ok = %v, want %v", a, op, b, ok, wantOK)
		return false
	}
	if ok && got.String() != want {
		tb.Logf("%s %s %s = %s, want %s", a, op, b, got, want)
		return false
	}
	return true
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

func rescaleOracle(d decimal.Decimal, scale uint8) (string, bool) {
	if scale > decimal.MaxScale {
		return "", false
	}

	negative := d.Sign() < 0
	r := decimalRat(d)
	if negative {
		r.Neg(r)
	}
	q, _ := roundRat(r, int(scale))
	if q.Cmp(mantissaBig(decimal.MaxMantissa())) > 0 {
		return "", false
	}
	return formatOracle(q, int(scale), negative), true
}

func roundOracle(d decimal.Decimal, scale uint8, mode decimal.RoundingMode) (string, bool) {
	if scale > decimal.MaxScale {
		return "", false
	}

	mantissa, curScale, negative := d.Parts()
	if mantissa == (decimal.Mantissa{}) || scale >= curScale {
		return d.String(), true
	}

	quotient, remainder, divisor := divModDroppedScale(mantissa, curScale-scale)
	if shouldRoundUpOracle(mode, negative, quotient, remainder, divisor) {
		quotient.Add(quotient, big.NewInt(1))
		if quotient.Cmp(mantissaBig(decimal.MaxMantissa())) > 0 {
			return "", false
		}
	}
	return formatOracle(quotient, int(scale), negative), true
}

func divModDroppedScale(mantissa decimal.Mantissa, drop uint8) (*big.Int, *big.Int, *big.Int) {
	divisor := pow10Big(int(drop))
	quotient, remainder := new(big.Int).QuoRem(mantissaBig(mantissa), divisor, new(big.Int))
	return quotient, remainder, divisor
}

func shouldRoundUpOracle(mode decimal.RoundingMode, negative bool, quotient, remainder, divisor *big.Int) bool {
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
	default:
		panic("decimal test: invalid rounding mode")
	}
}

func integralOracle(d decimal.Decimal, operation string) string {
	mantissa, scale, negative := d.Parts()
	if mantissa == (decimal.Mantissa{}) || scale == 0 {
		return d.String()
	}

	quotient, remainder, _ := divModDroppedScale(mantissa, scale)
	switch operation {
	case "floor":
		if negative && remainder.Sign() != 0 {
			quotient.Add(quotient, big.NewInt(1))
		}
	case "ceiling":
		if !negative && remainder.Sign() != 0 {
			quotient.Add(quotient, big.NewInt(1))
		}
	}
	return formatOracle(quotient, 0, negative)
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

func twice(n *big.Int) *big.Int {
	return new(big.Int).Lsh(n, 1)
}
