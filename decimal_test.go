package decimal_test

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/jacoelho/decimal"
)

func sameBits(a, b decimal.Decimal) bool {
	aLo, aMid, aHi, aFlags := a.Bits()
	bLo, bMid, bHi, bFlags := b.Bits()
	return aLo == bLo && aMid == bMid && aHi == bHi && aFlags == bFlags
}

func TestDecimalIsNotComparable(t *testing.T) {
	if reflect.TypeFor[decimal.Decimal]().Comparable() {
		t.Fatalf("Decimal should not be comparable; use Equal, Cmp, or Bits")
	}
}

func TestDecimalLimitsMatchDotNetDecimal(t *testing.T) {
	if decimal.MaxScale != 28 {
		t.Fatalf("MaxScale = %d, want 28", decimal.MaxScale)
	}
	if got := decimal.Max().String(); got != "79228162514264337593543950335" {
		t.Fatalf("Max() = %s", got)
	}
	if got := decimal.Min().String(); got != "-79228162514264337593543950335" {
		t.Fatalf("Min() = %s", got)
	}
	hi, lo := decimal.MaxMantissa().Words()
	if hi != math.MaxUint32 || lo != math.MaxUint64 {
		t.Fatalf("MaxMantissa().Words() = (%#x, %#x)", hi, lo)
	}
}

func TestNewValidatesParts(t *testing.T) {
	tests := []struct {
		name     string
		mantissa decimal.Mantissa
		scale    uint8
		want     error
	}{
		{
			name:     "scale above max",
			mantissa: decimal.MantissaFromUint64(1),
			scale:    decimal.MaxScale + 1,
			want:     decimal.ErrInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decimal.New(tt.mantissa, tt.scale, false)
			if !errors.Is(err, tt.want) {
				t.Fatalf("New() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNewAcceptsMaxMantissaAtMaxScale(t *testing.T) {
	d, err := decimal.New(decimal.MaxMantissa(), decimal.MaxScale, false)
	if err != nil {
		t.Fatalf("New(MaxMantissa, MaxScale) error: %v", err)
	}
	if got := d.String(); got != "7.9228162514264337593543950335" {
		t.Fatalf("New(MaxMantissa, MaxScale) = %s", got)
	}
}

func TestDotNetBitsRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		lo    uint32
		mid   uint32
		hi    uint32
		flags uint32
		want  string
	}{
		{name: "one", lo: 1, want: "1"},
		{name: "scaled", lo: 32610, flags: 4 << 16, want: "3.2610"},
		{name: "negative scaled", lo: 32610, flags: 4<<16 | 1<<31, want: "-3.2610"},
		{name: "max", lo: ^uint32(0), mid: ^uint32(0), hi: ^uint32(0), want: "79228162514264337593543950335"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := decimal.FromBits(tt.lo, tt.mid, tt.hi, tt.flags)
			if err != nil {
				t.Fatalf("FromBits error: %v", err)
			}
			if d.String() != tt.want {
				t.Fatalf("FromBits().String() = %s, want %s", d, tt.want)
			}
			lo, mid, hi, flags := d.Bits()
			if lo != tt.lo || mid != tt.mid || hi != tt.hi || flags != tt.flags {
				t.Fatalf("Bits() = (%#x, %#x, %#x, %#x), want (%#x, %#x, %#x, %#x)",
					lo, mid, hi, flags, tt.lo, tt.mid, tt.hi, tt.flags)
			}
		})
	}
}

func TestDotNetBitsUseLowMiddleHighWordOrder(t *testing.T) {
	const (
		lo    = uint32(0x01234567)
		mid   = uint32(0x89abcdef)
		hi    = uint32(0x13579bdf)
		scale = uint8(7)
		flags = uint32(scale)<<16 | uint32(1)<<31
	)

	d := decimal.MustFromBits(lo, mid, hi, flags)
	gotHi, gotLo := d.Mantissa().Words()
	wantLo := uint64(mid)<<32 | uint64(lo)
	if gotHi != hi || gotLo != wantLo {
		t.Fatalf("FromBits mantissa words = (%#x, %#x), want (%#x, %#x)", gotHi, gotLo, hi, wantLo)
	}

	constructed := decimal.MustNew(decimal.NewMantissa(hi, wantLo), scale, true)
	gotLoWord, gotMidWord, gotHiWord, gotFlags := constructed.Bits()
	if gotLoWord != lo || gotMidWord != mid || gotHiWord != hi || gotFlags != flags {
		t.Fatalf("Bits() = (%#x, %#x, %#x, %#x), want (%#x, %#x, %#x, %#x)",
			gotLoWord, gotMidWord, gotHiWord, gotFlags, lo, mid, hi, flags)
	}
	if !sameBits(d, constructed) {
		t.Fatalf("FromBits and MustNew constructed different representations")
	}
}

func TestFromBitsRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name  string
		flags uint32
	}{
		{name: "low reserved bit", flags: 1},
		{name: "high reserved bit", flags: 1 << 24},
		{name: "scale above max", flags: 29 << 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decimal.FromBits(1, 0, 0, tt.flags); !errors.Is(err, decimal.ErrInvalid) {
				t.Fatalf("FromBits error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestFromBitsCanonicalizesZero(t *testing.T) {
	d := decimal.MustFromBits(0, 0, 0, 28<<16|1<<31)
	if d.String() != "0" || d.Sign() != 0 || d.Scale() != 0 {
		t.Fatalf("zero not canonical: string=%q sign=%d scale=%d", d.String(), d.Sign(), d.Scale())
	}
	if lo, mid, hi, flags := d.Bits(); lo != 0 || mid != 0 || hi != 0 || flags != 0 {
		t.Fatalf("zero Bits() = (%#x, %#x, %#x, %#x)", lo, mid, hi, flags)
	}
}

func TestNewCanonicalizesZero(t *testing.T) {
	z := decimal.MustNew(decimal.Mantissa{}, 12, true)
	if z.String() != "0" || z.Sign() != 0 || z.Scale() != 0 {
		t.Fatalf("zero not canonical: string=%q sign=%d scale=%d", z.String(), z.Sign(), z.Scale())
	}
}

func TestPartsReturnsMantissaScaleAndSign(t *testing.T) {
	d := decimal.MustNew(decimal.MantissaFromUint64(12345), 2, true)
	m, scale, negative := d.Parts()
	if m != decimal.MantissaFromUint64(12345) || scale != 2 || !negative {
		t.Fatalf("Parts() = (%v, %d, %v)", m, scale, negative)
	}
}

func TestSignNegAndAbs(t *testing.T) {
	d := decimal.MustNew(decimal.MantissaFromUint64(12345), 2, true)
	if d.Sign() != -1 || !d.IsNegative() || d.IsPositive() || d.IsZero() {
		t.Fatalf("negative Sign=%d IsNegative=%v IsPositive=%v IsZero=%v", d.Sign(), d.IsNegative(), d.IsPositive(), d.IsZero())
	}
	if got := d.Neg(); got.String() != "123.45" || got.Sign() != 1 {
		t.Fatalf("Neg() = %s sign %d, want 123.45 sign 1", got, got.Sign())
	}
	if got := d.Abs(); got.String() != "123.45" || got.Sign() != 1 {
		t.Fatalf("Abs() = %s sign %d, want 123.45 sign 1", got, got.Sign())
	}

	var zero decimal.Decimal
	if !sameBits(zero.Neg(), zero) || !sameBits(zero.Abs(), zero) || zero.Sign() != 0 || zero.IsNegative() || zero.IsPositive() {
		t.Fatalf("zero unary result: Neg=%s Abs=%s Sign=%d IsNegative=%v IsPositive=%v",
			zero.Neg(), zero.Abs(), zero.Sign(), zero.IsNegative(), zero.IsPositive())
	}
}

func TestEqualIgnoresTrailingFractionalZeros(t *testing.T) {
	a := decimal.MustNew(decimal.MantissaFromUint64(1230), 3, false)
	b := decimal.MustNew(decimal.MantissaFromUint64(123), 2, false)
	if !a.Equal(b) || a.Cmp(b) != 0 {
		t.Fatalf("%s and %s should compare equal", a, b)
	}
	if sameBits(a, b) {
		t.Fatalf("%s and %s should not have identical representation", a, b)
	}
}

func TestCmpOrdersSignedAndScaledValues(t *testing.T) {
	tests := []struct {
		name string
		a    decimal.Decimal
		b    decimal.Decimal
		want int
	}{
		{"negative less than zero", decimal.FromInt64(-1), decimal.Decimal{}, -1},
		{"positive greater than zero", decimal.FromInt64(1), decimal.Decimal{}, 1},
		{"more negative is less", decimal.FromInt64(-2), decimal.FromInt64(-1), -1},
		{"scale up left would overflow", decimal.Max(), decimal.MustNew(decimal.MantissaFromUint64(1), decimal.MaxScale, false), 1},
		{"scale up right would overflow", decimal.MustNew(decimal.MantissaFromUint64(1), decimal.MaxScale, false), decimal.Max(), -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Cmp(tt.b); got != tt.want {
				t.Fatalf("%s.Cmp(%s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
			if got := tt.b.Cmp(tt.a); got != -tt.want {
				t.Fatalf("%s.Cmp(%s) = %d, want %d", tt.b, tt.a, got, -tt.want)
			}
		})
	}
}

func TestNormalizeTrimsTrailingFractionalZeros(t *testing.T) {
	a := decimal.MustNew(decimal.MantissaFromUint64(1230), 3, false)
	if got := a.Normalize().String(); got != "1.23" {
		t.Fatalf("Normalize() = %q, want 1.23", got)
	}
}

func TestRescaleRejectsInvalidScaleAndOverflow(t *testing.T) {
	if _, err := decimal.FromUint64(1).Rescale(decimal.MaxScale + 1); !errors.Is(err, decimal.ErrInvalid) {
		t.Fatalf("Rescale above MaxScale error = %v, want ErrInvalid", err)
	}
	if _, err := decimal.Max().Rescale(1); !errors.Is(err, decimal.ErrOverflow) {
		t.Fatalf("Rescale overflowing error = %v, want ErrOverflow", err)
	}
	if got, err := (decimal.Decimal{}).Rescale(2); err != nil || !got.IsZero() || got.Scale() != 0 {
		t.Fatalf("zero Rescale(2) = %s scale %d err %v, want canonical zero", got, got.Scale(), err)
	}
}

func TestIntegralOperations(t *testing.T) {
	tests := []struct {
		in       string
		truncate string
		floor    string
		ceiling  string
	}{
		{"1.23", "1", "1", "2"},
		{"-1.23", "-1", "-2", "-1"},
		{"1.00", "1", "1", "1"},
		{"-0.5", "0", "-1", "0"},
		{"0.5", "0", "0", "1"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			d := mustParse(t, tt.in)
			if got := d.Truncate().String(); got != tt.truncate {
				t.Fatalf("Truncate = %s, want %s", got, tt.truncate)
			}
			if got := d.Floor().String(); got != tt.floor {
				t.Fatalf("Floor = %s, want %s", got, tt.floor)
			}
			if got := d.Ceiling().String(); got != tt.ceiling {
				t.Fatalf("Ceiling = %s, want %s", got, tt.ceiling)
			}
		})
	}
}

func TestIsInteger(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"0", true},
		{"1", true},
		{"1.00", true},
		{"-1.0000", true},
		{"1.01", false},
		{"-0.1", false},
	}

	for _, tt := range tests {
		if got := mustParse(t, tt.in).IsInteger(); got != tt.want {
			t.Fatalf("%s.IsInteger() = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestMinMaxClamp(t *testing.T) {
	onePointTwoZero := mustParse(t, "1.20")
	onePointTwo := mustParse(t, "1.2")
	if got := onePointTwoZero.Min(onePointTwo); !sameBits(got, onePointTwoZero) {
		t.Fatalf("Min equal values = %s, want receiver representation", got)
	}
	if got := onePointTwoZero.Max(onePointTwo); !sameBits(got, onePointTwoZero) {
		t.Fatalf("Max equal values = %s, want receiver representation", got)
	}

	one := decimal.FromUint64(1)
	two := decimal.FromUint64(2)
	three := decimal.FromUint64(3)
	if got, err := two.Clamp(one, three); err != nil || !sameBits(got, two) {
		t.Fatalf("2.Clamp(1, 3) = %s err %v, want 2 nil", got, err)
	}
	if got, err := (decimal.Decimal{}).Clamp(one, three); err != nil || !sameBits(got, one) {
		t.Fatalf("0.Clamp(1, 3) = %s err %v, want 1 nil", got, err)
	}
	if got, err := decimal.FromUint64(5).Clamp(one, three); err != nil || !sameBits(got, three) {
		t.Fatalf("5.Clamp(1, 3) = %s err %v, want 3 nil", got, err)
	}
	if _, err := two.Clamp(three, one); !errors.Is(err, decimal.ErrInvalid) {
		t.Fatalf("Clamp with min > max error = %v, want ErrInvalid", err)
	}
}

func TestIntegerConversionsTruncateTowardZero(t *testing.T) {
	int64Tests := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"123.99", 123, true},
		{"-123.99", -123, true},
		{"-9223372036854775808.9", math.MinInt64, true},
		{"9223372036854775807.9", math.MaxInt64, true},
		{"9223372036854775808", 0, false},
		{"-9223372036854775809", 0, false},
	}
	for _, tt := range int64Tests {
		got, err := mustParse(t, tt.in).Int64()
		if got != tt.want || (err == nil) != tt.ok {
			t.Fatalf("%s.Int64() = %d err %v, want %d ok %v", tt.in, got, err, tt.want, tt.ok)
		}
		if !tt.ok && !errors.Is(err, decimal.ErrOverflow) {
			t.Fatalf("%s.Int64() error = %v, want ErrOverflow", tt.in, err)
		}
	}

	uint64Tests := []struct {
		in   string
		want uint64
		ok   bool
	}{
		{"123.99", 123, true},
		{"-0.5", 0, true},
		{"-1", 0, false},
		{"18446744073709551615.9", math.MaxUint64, true},
		{"18446744073709551616", 0, false},
	}
	for _, tt := range uint64Tests {
		got, err := mustParse(t, tt.in).Uint64()
		if got != tt.want || (err == nil) != tt.ok {
			t.Fatalf("%s.Uint64() = %d err %v, want %d ok %v", tt.in, got, err, tt.want, tt.ok)
		}
		if !tt.ok && !errors.Is(err, decimal.ErrOverflow) {
			t.Fatalf("%s.Uint64() error = %v, want ErrOverflow", tt.in, err)
		}
	}

	int32Tests := []struct {
		in   string
		want int32
		ok   bool
	}{
		{"2147483647.9", math.MaxInt32, true},
		{"-2147483648.9", math.MinInt32, true},
		{"2147483648", 0, false},
	}
	for _, tt := range int32Tests {
		got, err := mustParse(t, tt.in).Int32()
		if got != tt.want || (err == nil) != tt.ok {
			t.Fatalf("%s.Int32() = %d err %v, want %d ok %v", tt.in, got, err, tt.want, tt.ok)
		}
		if !tt.ok && !errors.Is(err, decimal.ErrOverflow) {
			t.Fatalf("%s.Int32() error = %v, want ErrOverflow", tt.in, err)
		}
	}

	uint32Tests := []struct {
		in   string
		want uint32
		ok   bool
	}{
		{"4294967295.9", math.MaxUint32, true},
		{"4294967296", 0, false},
	}
	for _, tt := range uint32Tests {
		got, err := mustParse(t, tt.in).Uint32()
		if got != tt.want || (err == nil) != tt.ok {
			t.Fatalf("%s.Uint32() = %d err %v, want %d ok %v", tt.in, got, err, tt.want, tt.ok)
		}
		if !tt.ok && !errors.Is(err, decimal.ErrOverflow) {
			t.Fatalf("%s.Uint32() error = %v, want ErrOverflow", tt.in, err)
		}
	}
}

func TestJSONMarshalUnmarshalQuotedDecimal(t *testing.T) {
	d := decimal.MustNew(decimal.MantissaFromUint64(1234500), 4, true)
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}
	if string(data) != `"-123.4500"` {
		t.Fatalf("MarshalJSON = %s", data)
	}
	var got decimal.Decimal
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if !got.Equal(d) || got.String() != d.String() {
		t.Fatalf("UnmarshalJSON = %s, want %s", got, d)
	}
}

func TestJSONUnmarshalAcceptsNumberAndRejectsInvalidInput(t *testing.T) {
	var got decimal.Decimal
	if err := json.Unmarshal([]byte(`123.45`), &got); err != nil {
		t.Fatalf("UnmarshalJSON number error: %v", err)
	}
	if got.String() != "123.45" {
		t.Fatalf("UnmarshalJSON number = %s, want 123.45", got)
	}
	if err := json.Unmarshal([]byte(`"not decimal"`), &got); !errors.Is(err, decimal.ErrParse) {
		t.Fatalf("UnmarshalJSON invalid error = %v, want ErrParse", err)
	}
}

func TestTextMarshalUnmarshalDecimal(t *testing.T) {
	d := decimal.MustNew(decimal.MantissaFromUint64(1234500), 4, true)
	data, err := d.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error: %v", err)
	}
	if string(data) != "-123.4500" {
		t.Fatalf("MarshalText = %q, want -123.4500", data)
	}

	var got decimal.Decimal
	if err := got.UnmarshalText(data); err != nil {
		t.Fatalf("UnmarshalText error: %v", err)
	}
	if !got.Equal(d) || got.String() != d.String() {
		t.Fatalf("UnmarshalText = %s, want %s", got, d)
	}
}

func TestAppendTextAppendsToExistingBuffer(t *testing.T) {
	d := decimal.MustNew(decimal.MantissaFromUint64(314), 2, false)
	got, err := d.AppendText([]byte("value="))
	if err != nil {
		t.Fatalf("AppendText error: %v", err)
	}
	if string(got) != "value=3.14" {
		t.Fatalf("AppendText = %q, want value=3.14", got)
	}
}

func TestSQLValueAndScanText(t *testing.T) {
	d := decimal.MustNew(decimal.MantissaFromUint64(314), 2, false)
	v, err := d.Value()
	if err != nil {
		t.Fatalf("Value error: %v", err)
	}
	if v != "3.14" {
		t.Fatalf("Value = %v", v)
	}

	var got decimal.Decimal
	if err := got.Scan([]byte("3.14")); err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if !got.Equal(d) {
		t.Fatalf("Scan = %s, want %s", got, d)
	}

	if err := got.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) error: %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("Scan(nil) = %s, want zero", got)
	}

	if err := got.Scan("3.14"); err != nil {
		t.Fatalf("Scan string error: %v", err)
	}
	if !got.Equal(d) {
		t.Fatalf("Scan string = %s, want %s", got, d)
	}
	if err := got.Scan(int64(-42)); err != nil {
		t.Fatalf("Scan int64 error: %v", err)
	}
	if want := decimal.FromInt64(-42); !got.Equal(want) {
		t.Fatalf("Scan int64 = %s, want %s", got, want)
	}
	if err := got.Scan(3.14); !errors.Is(err, decimal.ErrParse) {
		t.Fatalf("Scan unsupported error = %v, want ErrParse", err)
	}
}
