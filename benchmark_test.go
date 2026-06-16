package decimal_test

import (
	"strings"
	"testing"

	"github.com/jacoelho/decimal"
)

var (
	benchA               = mustBenchmarkParse("123456789.123456789")
	benchB               = mustBenchmarkParse("98765.4321")
	benchMaxString       = decimal.Max().String()
	benchMaxScaleTiny    = mustBenchmarkParse("0." + strings.Repeat("0", int(decimal.MaxScale)-1) + "1")
	benchMaxScaleNonzero = mustBenchmarkParse("0." + strings.Repeat("0", int(decimal.MaxScale)-1) + "9")
	sinkDecimal          decimal.Decimal
	sinkString           string
	sinkBytes            []byte
)

func BenchmarkParse(b *testing.B) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{name: "u64_scaled", input: "123456789.123456789"},
		{name: "max_mantissa", input: benchMaxString},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				d, err := decimal.Parse(tc.input)
				if err != nil {
					b.Fatal(err)
				}
				sinkDecimal = d
			}
		})
	}
}

func BenchmarkString(b *testing.B) {
	for _, tc := range []struct {
		name string
		d    decimal.Decimal
	}{
		{name: "u64_scaled", d: benchA},
		{name: "max_mantissa", d: decimal.Max()},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				sinkString = tc.d.String()
			}
		})
	}
}

func BenchmarkAppendText(b *testing.B) {
	for _, tc := range []struct {
		name string
		d    decimal.Decimal
	}{
		{name: "u64_scaled", d: benchA},
		{name: "max_mantissa", d: decimal.Max()},
	} {
		b.Run(tc.name, func(b *testing.B) {
			buf := make([]byte, 0, 80)
			for b.Loop() {
				buf = buf[:0]
				var err error
				sinkBytes, err = tc.d.AppendText(buf)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkAdd(b *testing.B) {
	for b.Loop() {
		var ok bool
		sinkDecimal, ok = benchA.Add(benchB)
		if !ok {
			b.Fatal("overflow")
		}
	}
}

func BenchmarkMul(b *testing.B) {
	cases := []struct {
		name string
		a, b decimal.Decimal
	}{
		{name: "exact", a: benchA, b: benchB},
		{name: "rounded", a: decimal.MustNew(decimal.MaxMantissa(), decimal.MaxScale, false), b: decimal.FromUint64(9)},
		{
			name: "max_scale_by_max_scale",
			a:    decimal.MustNew(decimal.MaxMantissa(), decimal.MaxScale, false),
			b:    decimal.MustNew(decimal.MaxMantissa(), decimal.MaxScale, false),
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				var ok bool
				sinkDecimal, ok = tc.a.Mul(tc.b)
				if !ok {
					b.Fatal("overflow")
				}
			}
		})
	}
}

func BenchmarkDiv(b *testing.B) {
	cases := []struct {
		name     string
		dividend decimal.Decimal
		divisor  decimal.Decimal
	}{
		{name: "baseline", dividend: benchA, divisor: benchB},
		{name: "by_one", dividend: benchA, divisor: decimal.FromUint64(1)},
		{name: "by_minus_one", dividend: benchA, divisor: decimal.FromInt64(-1)},
		{name: "exact", dividend: mustBenchmarkParse("123456789.123456789"), divisor: mustBenchmarkParse("0.000001")},
		{name: "repeating", dividend: decimal.FromUint64(1), divisor: decimal.FromUint64(3)},
		{name: "max_power_exact", dividend: decimal.FromUint64(1), divisor: decimal.MustNew(decimal.MantissaFromUint64(1), decimal.MaxScale, false)},
		{name: "max_power_noncanonical", dividend: decimal.FromUint64(1), divisor: decimal.MustNew(decimal.MantissaFromUint64(10), decimal.MaxScale, false)},
		{name: "max_power_repeating", dividend: decimal.FromUint64(1), divisor: decimal.MustNew(decimal.MantissaFromUint64(3), decimal.MaxScale, false)},
		{name: "max_power_96bit_divisor", dividend: decimal.FromUint64(1), divisor: decimal.MustNew(decimal.MaxMantissa(), decimal.MaxScale, false)},
		{name: "small_divisor", dividend: mustBenchmarkParse("123456789123456789.123456789"), divisor: decimal.FromUint64(7)},
		{name: "large_divisor", dividend: benchA, divisor: mustBenchmarkParse("987654321987654321.123456789")},
		{name: "max_mantissa_large_divisor", dividend: decimal.Max(), divisor: mustBenchmarkParse("987654321987654321.123456789")},
		{name: "underflow_zero", dividend: benchMaxScaleTiny, divisor: decimal.FromUint64(9)},
		{name: "max_scale_nonzero", dividend: benchMaxScaleNonzero, divisor: decimal.FromUint64(3)},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				var ok bool
				sinkDecimal, ok = tc.dividend.Div(tc.divisor)
				if !ok {
					b.Fatal("overflow")
				}
			}
		})
	}
}

func BenchmarkRemainder(b *testing.B) {
	for _, tc := range []struct {
		name     string
		dividend decimal.Decimal
		divisor  decimal.Decimal
	}{
		{name: "baseline", dividend: benchA, divisor: benchB},
		{name: "small_divisor", dividend: mustBenchmarkParse("123456789123456789.123456789"), divisor: decimal.FromUint64(7)},
		{name: "common_scale", dividend: mustBenchmarkParse("5.00"), divisor: mustBenchmarkParse("2.00")},
		{name: "scaled_divisor_overflow", dividend: decimal.MustNew(decimal.MaxMantissa(), decimal.MaxScale, false), divisor: decimal.Max()},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				var ok bool
				sinkDecimal, ok = tc.dividend.Remainder(tc.divisor)
				if !ok {
					b.Fatal("remainder failed")
				}
			}
		})
	}
}

func BenchmarkRound(b *testing.B) {
	value := mustBenchmarkParse("123456789.123456789")
	for _, tc := range []struct {
		name  string
		scale uint8
		mode  decimal.RoundingMode
	}{
		{name: "half_away", scale: 2, mode: decimal.RoundHalfAwayFromZero},
		{name: "half_even", scale: 2, mode: decimal.RoundHalfToEven},
		{name: "toward_zero", scale: 2, mode: decimal.RoundTowardZero},
		{name: "toward_positive_infinity", scale: 2, mode: decimal.RoundTowardPositiveInfinity},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				var ok bool
				sinkDecimal, ok = value.Round(tc.scale, tc.mode)
				if !ok {
					b.Fatal("round failed")
				}
			}
		})
	}
}

func BenchmarkRescale(b *testing.B) {
	value := mustBenchmarkParse("123456789.123456789")
	for _, tc := range []struct {
		name  string
		scale uint8
	}{
		{name: "reduce_one_digit", scale: 8},
		{name: "reduce_many_digits", scale: 2},
		{name: "increase_scale", scale: 12},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				var ok bool
				sinkDecimal, ok = value.Rescale(tc.scale)
				if !ok {
					b.Fatal("rescale failed")
				}
			}
		})
	}
}

func BenchmarkNormalize(b *testing.B) {
	for _, tc := range []struct {
		name string
		d    decimal.Decimal
	}{
		{name: "no_trailing_zero", d: benchA},
		{name: "u64_many_trailing_zeros", d: mustBenchmarkParse("123456789.000000000")},
		{name: "max_mantissa_max_scale", d: decimal.MustNew(decimal.MaxMantissa(), decimal.MaxScale, false)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				sinkDecimal = tc.d.Normalize()
			}
		})
	}
}

func mustBenchmarkParse(s string) decimal.Decimal {
	d, err := decimal.Parse(s)
	if err != nil {
		panic(err)
	}
	return d
}
