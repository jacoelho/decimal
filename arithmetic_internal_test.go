package decimal

import (
	"math/big"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
)

var (
	two96    = new(big.Int).Lsh(big.NewInt(1), 96)
	two192   = new(big.Int).Lsh(big.NewInt(1), 192)
	two384   = new(big.Int).Lsh(big.NewInt(1), 384)
	maxBig96 = new(big.Int).Sub(new(big.Int).Set(two96), big.NewInt(1))
	maxWord  = new(big.Int).SetUint64(^uint64(0))
)

func (Mantissa) Generate(r *rand.Rand, _ int) reflect.Value {
	return reflect.ValueOf(NewMantissa(r.Uint32(), r.Uint64()))
}

func (uint192) Generate(r *rand.Rand, _ int) reflect.Value {
	return reflect.ValueOf(uint192{
		hi:  r.Uint64(),
		mid: r.Uint64(),
		lo:  r.Uint64(),
	})
}

func (uint384) Generate(r *rand.Rand, _ int) reflect.Value {
	return reflect.ValueOf(uint384{w: [6]uint64{
		r.Uint64(),
		r.Uint64(),
		r.Uint64(),
		r.Uint64(),
		r.Uint64(),
		r.Uint64(),
	}})
}

func TestMaxMantissaDecimalMatchesMaxMantissa(t *testing.T) {
	if got := Max().String(); got != maxMantissaDecimal {
		t.Fatalf("maxMantissaDecimal = %q, want %q", maxMantissaDecimal, got)
	}
}

func TestDecimalStringMatchesBigIntOracle(t *testing.T) {
	values := []Mantissa{
		{},
		oneMantissa,
		MantissaFromUint64(10),
		MantissaFromUint64(^uint64(0)),
		NewMantissa(1, 0),
		NewMantissa(1234567890, 9876543210123456789),
		MaxMantissa(),
	}
	scales := []uint8{0, 1, 2, 9, 19, MaxScale}

	rng := rand.New(rand.NewSource(2)) //nolint:gosec // Deterministic test data, not cryptographic randomness.
	for range 2048 {
		values = append(values, NewMantissa(rng.Uint32(), rng.Uint64()))
	}

	for _, mantissa := range values {
		for _, scale := range scales {
			for _, negative := range []bool{false, true} {
				d := pack(negative, scale, mantissa)
				want := oracleDecimalString(mantissa, scale, negative)
				if got := d.String(); got != want {
					t.Fatalf("String(%v, scale=%d, negative=%v) = %q, want %q",
						mantissa, scale, negative, got, want)
				}

				got, err := d.AppendText([]byte("x="))
				if err != nil {
					t.Fatalf("AppendText error: %v", err)
				}
				if string(got) != "x="+want {
					t.Fatalf("AppendText(%v, scale=%d, negative=%v) = %q, want %q",
						mantissa, scale, negative, string(got), "x="+want)
				}
			}
		}
	}
}

func TestMantissaMulCheckedReportsOverflow(t *testing.T) {
	_, overflow := MaxMantissa().mulChecked(MantissaFromUint64(2))
	if !overflow {
		t.Fatalf("mulChecked overflow = false, want true")
	}
}

func TestMantissaCheckedArithmeticMatchesBigInt(t *testing.T) {
	if err := quick.Check(func(a, b Mantissa) bool {
		got, overflow := a.addChecked(b)
		want := new(big.Int).Add(bigMantissa(a), bigMantissa(b))
		if overflow != (want.Cmp(two96) >= 0) {
			return false
		}
		if !overflow && got != mantissaFromBigWrapped(want) {
			return false
		}

		got, overflow = a.mulChecked(b)
		want.Mul(bigMantissa(a), bigMantissa(b))
		if overflow != (want.Cmp(two96) >= 0) {
			return false
		}
		return overflow || got == mantissaFromBigWrapped(want)
	}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestFullMul96MatchesBigInt(t *testing.T) {
	values := []Mantissa{
		{},
		oneMantissa,
		MantissaFromUint64(2),
		MantissaFromUint64(^uint64(0)),
		NewMantissa(1, 0),
		NewMantissa(^uint32(0), 0),
		MaxMantissa(),
	}
	for _, a := range values {
		for _, b := range values {
			got := fullMul96(a, b)
			want := new(big.Int).Mul(bigMantissa(a), bigMantissa(b))
			if bigUint192(got).Cmp(want) != 0 {
				t.Fatalf("fullMul96(%s, %s) = %s, want %s",
					bigMantissa(a), bigMantissa(b), bigUint192(got), want)
			}
		}
	}

	cfg := &quick.Config{MaxCount: 2048}
	if err := quick.Check(func(a, b Mantissa) bool {
		got := fullMul96(a, b)
		want := new(big.Int).Mul(bigMantissa(a), bigMantissa(b))
		return bigUint192(got).Cmp(want) == 0
	}, cfg); err != nil {
		t.Fatal(err)
	}
}

func TestFullMul96By64MatchesBigInt(t *testing.T) {
	values := []Mantissa{
		{},
		oneMantissa,
		MantissaFromUint64(2),
		MantissaFromUint64(^uint64(0)),
		NewMantissa(1, 0),
		MaxMantissa(),
	}
	multipliers := []uint64{0, 1, 10, ^uint64(0)}

	rng := rand.New(rand.NewSource(7)) //nolint:gosec // Deterministic test data, not cryptographic randomness.
	for range 512 {
		values = append(values, NewMantissa(rng.Uint32(), rng.Uint64()))
		multipliers = append(multipliers, rng.Uint64())
	}

	for i, value := range values {
		for j, multiplier := range multipliers {
			got := fullMul96By64(value, multiplier)
			want := new(big.Int).Mul(bigMantissa(value), new(big.Int).SetUint64(multiplier))
			if bigUint192(got).Cmp(want) != 0 {
				t.Fatalf("values[%d] * multipliers[%d] = %s, want %s",
					i, j, bigUint192(got), want)
			}
		}
	}
}

func TestMantissaDivMod64MatchesBigInt(t *testing.T) {
	if err := quick.Check(func(a Mantissa, divisor uint64) bool {
		divisor |= 1
		gotQ, gotR := a.divMod64(divisor)
		wantQ, wantR := new(big.Int).QuoRem(bigMantissa(a), new(big.Int).SetUint64(divisor), new(big.Int))
		return gotQ == mantissaFromBigWrapped(wantQ) && gotR == wantR.Uint64()
	}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestDivMod10MatchesBigInt(t *testing.T) {
	mantissas := []Mantissa{
		{},
		oneMantissa,
		MantissaFromUint64(9),
		MantissaFromUint64(10),
		MantissaFromUint64(^uint64(0)),
		NewMantissa(1, 0),
		MaxMantissa(),
	}
	values := []uint192{
		{},
		{lo: 1},
		{lo: 9},
		{lo: 10},
		{lo: ^uint64(0)},
		{mid: 1},
		{hi: 1},
		{hi: ^uint64(0), mid: ^uint64(0), lo: ^uint64(0)},
	}
	rng := rand.New(rand.NewSource(5)) //nolint:gosec // Deterministic test data, not cryptographic randomness.
	for range 512 {
		mantissas = append(mantissas, NewMantissa(rng.Uint32(), rng.Uint64()))
		values = append(values, uint192{
			hi:  rng.Uint64(),
			mid: rng.Uint64(),
			lo:  rng.Uint64(),
		})
	}

	for i, value := range mantissas {
		gotQ, gotR := value.divMod64(10)
		wantQ, wantR := new(big.Int).QuoRem(bigMantissa(value), big.NewInt(10), new(big.Int))
		if bigMantissa(gotQ).Cmp(wantQ) != 0 || gotR != wantR.Uint64() {
			t.Fatalf("mantissas[%d]/10 = q %s r %d, want q %s r %s",
				i, bigMantissa(gotQ), gotR, wantQ, wantR)
		}
	}

	for i, value := range values {
		gotQ, gotR := value.divModSmall(10)
		wantQ, wantR := new(big.Int).QuoRem(bigUint192(value), big.NewInt(10), new(big.Int))
		if bigUint192(gotQ).Cmp(wantQ) != 0 || gotR != wantR.Uint64() {
			t.Fatalf("values[%d]/10 = q %s r %d, want q %s r %s",
				i, bigUint192(gotQ), gotR, wantQ, wantR)
		}
	}
}

func TestMantissaDivMod1e9MatchesBigInt(t *testing.T) {
	values := []Mantissa{
		{},
		oneMantissa,
		MantissaFromUint64(999_999_999),
		MantissaFromUint64(1_000_000_000),
		MantissaFromUint64(^uint64(0)),
		NewMantissa(1, 0),
		MaxMantissa(),
	}
	rng := rand.New(rand.NewSource(6)) //nolint:gosec // Deterministic test data, not cryptographic randomness.
	for range 512 {
		values = append(values, NewMantissa(rng.Uint32(), rng.Uint64()))
	}

	divisor := big.NewInt(1_000_000_000)
	for i, value := range values {
		gotQ, gotR := value.divMod1e9()
		wantQ, wantR := new(big.Int).QuoRem(bigMantissa(value), divisor, new(big.Int))
		if bigMantissa(gotQ).Cmp(wantQ) != 0 || uint64(gotR) != wantR.Uint64() {
			t.Fatalf("values[%d]/1e9 = q %s r %d, want q %s r %s",
				i, bigMantissa(gotQ), gotR, wantQ, wantR)
		}
	}
}

func TestUint192ScratchArithmeticWithBigIntOracle(t *testing.T) {
	values := []uint192{
		{},
		{lo: 1},
		{lo: ^uint64(0)},
		{mid: maxMantissaHiWord, lo: ^uint64(0)},
		{mid: 1},
		{hi: 1},
		{hi: ^uint64(0), mid: ^uint64(0), lo: ^uint64(0)},
	}
	divisors := []Mantissa{
		oneMantissa,
		MantissaFromUint64(3),
		MantissaFromUint64(^uint64(0)),
		NewMantissa(1, 0),
		MaxMantissa(),
	}

	rng := rand.New(rand.NewSource(1)) //nolint:gosec // Deterministic test data, not cryptographic randomness.
	for range 200 {
		values = append(values, uint192{
			hi:  rng.Uint64(),
			mid: rng.Uint64(),
			lo:  rng.Uint64(),
		})
		divisors = append(divisors, nonZeroMantissa(NewMantissa(rng.Uint32(), rng.Uint64())))
	}

	for i, a := range values {
		for j, b := range values {
			assertUint192Add(t, i, j, a, b)
			assertUint192Sub(t, i, j, a, b)
		}
		for j, divisor := range divisors {
			assertUint192DivMod(t, i, j, a, divisor)
		}
		assertUint192Mul96(t, i, a, divisors[(i*37+11)%len(divisors)])
		assertUint192Mul64(t, i, a, rng.Uint64())
		assertUint192DivModSmall(t, i, a, rng.Uint64()|1)
	}
}

func TestUint384ScratchArithmeticWithBigIntOracle(t *testing.T) {
	values := []uint384{
		{},
		{w: [6]uint64{1}},
		{w: [6]uint64{^uint64(0)}},
		{w: [6]uint64{0, 1}},
		{w: [6]uint64{0, 0, 0, 0, 0, 1}},
		{w: [6]uint64{^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0)}},
	}
	rng := rand.New(rand.NewSource(8)) //nolint:gosec // Deterministic test data, not cryptographic randomness.
	for range 256 {
		values = append(values, uint384{w: [6]uint64{
			rng.Uint64(),
			rng.Uint64(),
			rng.Uint64(),
			rng.Uint64(),
			rng.Uint64(),
			rng.Uint64(),
		}})
	}

	scales := []int{0, 1, 9, 28, 56}
	for i, value := range values {
		for _, scale := range scales {
			got, ok := value.scaleUp10(scale)
			want := new(big.Int).Mul(bigUint384(value), bigPow10(scale))
			wantOK := want.Cmp(two384) < 0
			if ok != wantOK {
				t.Fatalf("values[%d].scaleUp10(%d) ok = %v, want %v", i, scale, ok, wantOK)
			}
			if ok && bigUint384(got).Cmp(want) != 0 {
				t.Fatalf("values[%d].scaleUp10(%d) = %s, want %s", i, scale, bigUint384(got), want)
			}
		}

		m := NewMantissa(rng.Uint32(), rng.Uint64())
		gotMul, overflow := value.mulMantissa(m)
		wantMul := new(big.Int).Mul(bigUint384(value), bigMantissa(m))
		wantOverflow := wantMul.Cmp(two384) >= 0
		if overflow != wantOverflow {
			t.Fatalf("values[%d].mulMantissa(%s) overflow = %v, want %v", i, bigMantissa(m), overflow, wantOverflow)
		}
		if !overflow && bigUint384(gotMul).Cmp(wantMul) != 0 {
			t.Fatalf("values[%d].mulMantissa(%s) = %s, want %s", i, bigMantissa(m), bigUint384(gotMul), wantMul)
		}
	}

	for i, numerator := range values {
		divisor := values[(i*17+5)%len(values)]
		if divisor.isZero() {
			divisor = uint384{w: [6]uint64{1}}
		}
		gotQ, gotR := numerator.divMod(divisor)
		wantQ, wantR := new(big.Int).QuoRem(bigUint384(numerator), bigUint384(divisor), new(big.Int))
		if bigUint384(gotQ).Cmp(wantQ) != 0 || bigUint384(gotR).Cmp(wantR) != 0 {
			t.Fatalf("values[%d].divMod(divisor) = q %s r %s, want q %s r %s",
				i, bigUint384(gotQ), bigUint384(gotR), wantQ, wantR)
		}
	}
}

func TestScaleUpMatchesBigInt(t *testing.T) {
	values := []Mantissa{
		{},
		oneMantissa,
		MantissaFromUint64(10),
		MantissaFromUint64(^uint64(0)),
		NewMantissa(1, 0),
		MaxMantissa(),
	}
	rng := rand.New(rand.NewSource(8)) //nolint:gosec // Deterministic test data, not cryptographic randomness.
	for range 512 {
		values = append(values, NewMantissa(rng.Uint32(), rng.Uint64()))
	}

	for i, value := range values {
		for scale := range 57 {
			got, ok := scaleUp(value, scale)
			want := new(big.Int).Mul(bigMantissa(value), bigPow10(scale))
			wantOK := want.Cmp(two192) < 0
			if ok != wantOK {
				t.Fatalf("values[%d] scale %d ok = %v, want %v", i, scale, ok, wantOK)
			}
			if ok && bigUint192(got).Cmp(want) != 0 {
				t.Fatalf("values[%d] scale %d = %s, want %s",
					i, scale, bigUint192(got), want)
			}
		}
	}
}

func TestDivModPow10MantissaMatchesBigInt(t *testing.T) {
	values := []Mantissa{
		{},
		oneMantissa,
		MantissaFromUint64(9),
		MantissaFromUint64(10),
		MantissaFromUint64(^uint64(0)),
		NewMantissa(1, 0),
		NewMantissa(^uint32(0), ^uint64(0)),
	}
	rng := rand.New(rand.NewSource(4)) //nolint:gosec // Deterministic test data, not cryptographic randomness.
	for range 512 {
		values = append(values, NewMantissa(rng.Uint32(), rng.Uint64()))
	}

	for i, value := range values {
		for scale := uint8(0); scale <= MaxScale; scale++ {
			gotQ, gotR := divModPow10Mantissa(value, scale)
			wantQ, wantR := new(big.Int).QuoRem(bigMantissa(value), bigPow10(int(scale)), new(big.Int))
			if bigMantissa(gotQ).Cmp(wantQ) != 0 || bigMantissa(gotR).Cmp(wantR) != 0 {
				t.Fatalf("values[%d] / 10^%d = q %s r %s, want q %s r %s",
					i, scale, bigMantissa(gotQ), bigMantissa(gotR), wantQ, wantR)
			}
		}
	}
}

func TestDivMod192By96MatchesBigIntQuick(t *testing.T) {
	cfg := &quick.Config{MaxCount: 2048}
	if err := quick.Check(func(a uint192, b Mantissa) bool {
		b = nonZeroMantissa(b)
		gotQ, gotR := divMod192By96(a, b)
		wantQ, wantR := new(big.Int).QuoRem(bigUint192(a), bigMantissa(b), new(big.Int))
		return bigUint192(gotQ).Cmp(wantQ) == 0 && bigMantissa(gotR).Cmp(wantR) == 0
	}, cfg); err != nil {
		t.Fatal(err)
	}
}

func assertUint192Add(t *testing.T, i, j int, a, b uint192) {
	t.Helper()
	got, ok := a.add(b)
	want := new(big.Int).Add(bigUint192(a), bigUint192(b))
	wantOK := want.Cmp(two192) < 0
	if ok != wantOK {
		t.Fatalf("values[%d].add(values[%d]) ok = %v, want %v", i, j, ok, wantOK)
	}
	if ok && bigUint192(got).Cmp(want) != 0 {
		t.Fatalf("values[%d].add(values[%d]) = %s, want %s", i, j, bigUint192(got), want)
	}
}

func assertUint192Sub(t *testing.T, i, j int, a, b uint192) {
	t.Helper()
	got, ok := a.sub(b)
	aBig := bigUint192(a)
	bBig := bigUint192(b)
	wantOK := aBig.Cmp(bBig) >= 0
	if ok != wantOK {
		t.Fatalf("values[%d].sub(values[%d]) ok = %v, want %v", i, j, ok, wantOK)
	}
	if ok {
		want := new(big.Int).Sub(aBig, bBig)
		if bigUint192(got).Cmp(want) != 0 {
			t.Fatalf("values[%d].sub(values[%d]) = %s, want %s", i, j, bigUint192(got), want)
		}
	}
}

func assertUint192Mul96(t *testing.T, i int, a uint192, b Mantissa) {
	t.Helper()
	got, ok := a.mul96(b)
	want := new(big.Int).Mul(bigUint192(a), bigMantissa(b))
	wantOK := want.Cmp(two192) < 0
	if ok != wantOK {
		t.Fatalf("values[%d].mul96(%s) ok = %v, want %v", i, bigMantissa(b), ok, wantOK)
	}
	if ok && bigUint192(got).Cmp(want) != 0 {
		t.Fatalf("values[%d].mul96(%s) = %s, want %s", i, bigMantissa(b), bigUint192(got), want)
	}
}

func assertUint192Mul64(t *testing.T, i int, a uint192, b uint64) {
	t.Helper()
	got, ok := a.mul64(b)
	want := new(big.Int).Mul(bigUint192(a), new(big.Int).SetUint64(b))
	wantOK := want.Cmp(two192) < 0
	if ok != wantOK {
		t.Fatalf("values[%d].mul64(%d) ok = %v, want %v", i, b, ok, wantOK)
	}
	if ok && bigUint192(got).Cmp(want) != 0 {
		t.Fatalf("values[%d].mul64(%d) = %s, want %s", i, b, bigUint192(got), want)
	}
}

func assertUint192DivMod(t *testing.T, i, j int, a uint192, b Mantissa) {
	t.Helper()
	gotQ, gotR := divMod192By96(a, b)
	wantQ, wantR := new(big.Int).QuoRem(bigUint192(a), bigMantissa(b), new(big.Int))
	if bigUint192(gotQ).Cmp(wantQ) != 0 || bigMantissa(gotR).Cmp(wantR) != 0 {
		t.Fatalf("values[%d] / divisor[%d] = q %s r %s, want q %s r %s",
			i, j, bigUint192(gotQ), bigMantissa(gotR), wantQ, wantR)
	}
}

func assertUint192DivModSmall(t *testing.T, i int, a uint192, divisor uint64) {
	t.Helper()
	gotQ, gotR := a.divModSmall(divisor)
	wantQ, wantR := new(big.Int).QuoRem(bigUint192(a), new(big.Int).SetUint64(divisor), new(big.Int))
	if bigUint192(gotQ).Cmp(wantQ) != 0 || gotR != wantR.Uint64() {
		t.Fatalf("values[%d].divModSmall(%d) = q %s r %d, want q %s r %s",
			i, divisor, bigUint192(gotQ), gotR, wantQ, wantR)
	}
}

func nonZeroMantissa(v Mantissa) Mantissa {
	if v.isZero() {
		return oneMantissa
	}
	return v
}

func bigUint192(n uint192) *big.Int {
	hi := new(big.Int).SetUint64(n.hi)
	hi.Lsh(hi, 64)
	hi.Add(hi, new(big.Int).SetUint64(n.mid))
	hi.Lsh(hi, 64)
	return hi.Add(hi, new(big.Int).SetUint64(n.lo))
}

func bigUint384(n uint384) *big.Int {
	out := new(big.Int)
	for i := len(n.w) - 1; i >= 0; i-- {
		out.Lsh(out, 64)
		out.Add(out, new(big.Int).SetUint64(n.w[i]))
	}
	return out
}

func bigPow10(exp int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp)), nil)
}

func bigMantissa(m Mantissa) *big.Int {
	hiWord, loWord := m.Words()
	hi := new(big.Int).SetUint64(uint64(hiWord))
	hi.Lsh(hi, 64)
	return hi.Add(hi, new(big.Int).SetUint64(loWord))
}

func oracleDecimalString(m Mantissa, scale uint8, negative bool) string {
	if m.isZero() {
		return "0"
	}

	digits := bigMantissa(m).String()
	if scale > 0 {
		places := int(scale)
		if len(digits) <= places {
			digits = "0." + strings.Repeat("0", places-len(digits)) + digits
		} else {
			split := len(digits) - places
			digits = digits[:split] + "." + digits[split:]
		}
	}
	if negative {
		return "-" + digits
	}
	return digits
}

func mantissaFromBigWrapped(n *big.Int) Mantissa {
	wrapped := new(big.Int).And(new(big.Int).Set(n), maxBig96)
	hi := new(big.Int).Rsh(wrapped, 64)
	lo := new(big.Int).And(wrapped, maxWord)
	return NewMantissa(uint32FromChecked(hi.Uint64()), lo.Uint64())
}
