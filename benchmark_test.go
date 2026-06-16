package decimal_test

import (
	"errors"
	"fmt"
	"io"
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

func BenchmarkFormatPadding(b *testing.B) {
	value := mustBenchmarkParse("123456789.12")
	for _, tc := range []struct {
		name   string
		format string
	}{
		{name: "space", format: "%100.2f"},
		{name: "zero", format: "%0100.2f"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				if _, err := fmt.Fprintf(io.Discard, tc.format, value); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkFormatStringVerbs(b *testing.B) {
	value := mustBenchmarkParse("123456789.12")
	for _, tc := range []struct {
		name   string
		format string
	}{
		{name: "s", format: "%s"},
		{name: "v", format: "%v"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				if _, err := fmt.Fprintf(io.Discard, tc.format, value); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkAdd(b *testing.B) {
	for b.Loop() {
		var err error
		sinkDecimal, err = benchA.Add(benchB)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSub(b *testing.B) {
	for b.Loop() {
		var err error
		sinkDecimal, err = benchA.Sub(benchB)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMul(b *testing.B) {
	cases := []struct {
		name string
		a, b decimal.Decimal
	}{
		{name: "exact", a: benchA, b: benchB},
		{name: "scale_reduces_exactly", a: mustBenchmarkParse("1.2300"), b: mustBenchmarkParse("2.00")},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				var err error
				sinkDecimal, err = tc.a.Mul(tc.b)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkMulToScale(b *testing.B) {
	cases := []struct {
		name  string
		a, b  decimal.Decimal
		scale uint8
	}{
		{name: "cents", a: benchA, b: benchB, scale: 2},
		{name: "exact_scale", a: mustBenchmarkParse("1.2300"), b: mustBenchmarkParse("2.00"), scale: 4},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				var err error
				sinkDecimal, err = tc.a.MulToScale(tc.b, tc.scale, decimal.RoundHalfAwayFromZero)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkMulQuantized(b *testing.B) {
	quantum := mustBenchmarkParse("0.05")
	for b.Loop() {
		var err error
		sinkDecimal, err = benchA.MulQuantized(benchB, quantum, decimal.RoundHalfAwayFromZero)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDiv(b *testing.B) {
	cases := []struct {
		name     string
		dividend decimal.Decimal
		divisor  decimal.Decimal
		wantErr  error
	}{
		{name: "by_one", dividend: benchA, divisor: decimal.FromUint64(1)},
		{name: "by_minus_one", dividend: benchA, divisor: decimal.FromInt64(-1)},
		{name: "by_ten", dividend: benchA, divisor: decimal.FromUint64(10)},
		{name: "exact", dividend: mustBenchmarkParse("123456789.123456789"), divisor: mustBenchmarkParse("0.000001")},
		{name: "one_eighth", dividend: decimal.FromUint64(1), divisor: decimal.FromUint64(8)},
		{name: "one_fourth", dividend: decimal.FromUint64(1), divisor: decimal.FromUint64(4)},
		{name: "one_third_inexact", dividend: decimal.FromUint64(1), divisor: decimal.FromUint64(3), wantErr: decimal.ErrInexact},
		{name: "near_max_overflow", dividend: maxMinus(7), divisor: mustBenchmarkParse("0.9999999999999999999999999999"), wantErr: decimal.ErrOverflow},
		{name: "max_power_exact", dividend: decimal.FromUint64(1), divisor: decimal.MustNew(decimal.MantissaFromUint64(1), decimal.MaxScale, false)},
		{name: "max_power_noncanonical", dividend: decimal.FromUint64(1), divisor: decimal.MustNew(decimal.MantissaFromUint64(10), decimal.MaxScale, false)},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				var err error
				sinkDecimal, err = tc.dividend.Div(tc.divisor)
				if tc.wantErr != nil {
					if !errors.Is(err, tc.wantErr) {
						b.Fatalf("Div error = %v, want %v", err, tc.wantErr)
					}
					continue
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDivQuantized(b *testing.B) {
	quantum := mustBenchmarkParse("0.0001")
	for b.Loop() {
		var err error
		sinkDecimal, err = benchA.DivQuantized(benchB, quantum, decimal.RoundHalfAwayFromZero)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDivToScale(b *testing.B) {
	for _, tc := range []struct {
		name     string
		dividend decimal.Decimal
		divisor  decimal.Decimal
		scale    uint8
	}{
		{name: "repeating", dividend: decimal.FromUint64(1), divisor: decimal.FromUint64(3), scale: decimal.MaxScale},
		{name: "small_divisor", dividend: mustBenchmarkParse("123456789123456789.123456789"), divisor: decimal.FromUint64(7), scale: 6},
		{name: "underflow_zero", dividend: benchMaxScaleTiny, divisor: decimal.FromUint64(9), scale: decimal.MaxScale},
		{name: "max_scale_nonzero", dividend: benchMaxScaleNonzero, divisor: decimal.FromUint64(3), scale: decimal.MaxScale},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				var err error
				sinkDecimal, err = tc.dividend.DivToScale(tc.divisor, tc.scale, decimal.RoundHalfAwayFromZero)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkMulAddToScale(b *testing.B) {
	cases := []struct {
		name              string
		value, multiplier decimal.Decimal
		addend            decimal.Decimal
		scale             uint8
	}{
		{name: "same_sign", value: benchA, multiplier: benchB, addend: mustBenchmarkParse("12.34"), scale: 2},
		{name: "subtract_addend", value: benchA, multiplier: benchB.Neg(), addend: mustBenchmarkParse("12.34"), scale: 2},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				var err error
				sinkDecimal, err = tc.value.MulAddToScale(tc.multiplier, tc.addend, tc.scale, decimal.RoundHalfAwayFromZero)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkMulAddQuantized(b *testing.B) {
	quantum := mustBenchmarkParse("0.05")
	addend := mustBenchmarkParse("12.34")
	for b.Loop() {
		var err error
		sinkDecimal, err = benchA.MulAddQuantized(benchB, addend, quantum, decimal.RoundHalfAwayFromZero)
		if err != nil {
			b.Fatal(err)
		}
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
				var err error
				sinkDecimal, err = tc.dividend.Remainder(tc.divisor)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkQuantize(b *testing.B) {
	value := mustBenchmarkParse("123456789.123456789")
	for _, tc := range []struct {
		name  string
		scale uint8
		mode  decimal.RoundingMode
	}{
		{name: "half_away", scale: 2, mode: decimal.RoundHalfAwayFromZero},
		{name: "half_even", scale: 2, mode: decimal.RoundHalfToEven},
		{name: "toward_zero", scale: 2, mode: decimal.RoundTowardZero},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				var err error
				sinkDecimal, err = value.Quantize(tc.scale, tc.mode)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkQuantizeTo(b *testing.B) {
	value := mustBenchmarkParse("123456789.123456789")
	for _, tc := range []struct {
		name    string
		quantum decimal.Decimal
	}{
		{name: "cent", quantum: mustBenchmarkParse("0.01")},
		{name: "nickel", quantum: mustBenchmarkParse("0.05")},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				var err error
				sinkDecimal, err = value.QuantizeTo(tc.quantum, decimal.RoundHalfAwayFromZero)
				if err != nil {
					b.Fatal(err)
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
				var err error
				sinkDecimal, err = value.Round(tc.scale, tc.mode)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkTruncate(b *testing.B) {
	value := mustBenchmarkParse("-123456789.123456789")
	for b.Loop() {
		sinkDecimal = value.Truncate()
	}
}

func BenchmarkFloor(b *testing.B) {
	value := mustBenchmarkParse("-123456789.123456789")
	for b.Loop() {
		sinkDecimal = value.Floor()
	}
}

func BenchmarkCeiling(b *testing.B) {
	value := mustBenchmarkParse("123456789.123456789")
	for b.Loop() {
		sinkDecimal = value.Ceiling()
	}
}

func BenchmarkIsInteger(b *testing.B) {
	value := mustBenchmarkParse("123456789.000000000")
	var sinkBool bool
	for b.Loop() {
		sinkBool = value.IsInteger()
	}
	if sinkBool {
		sinkDecimal = value
	}
}

func BenchmarkRescale(b *testing.B) {
	value := mustBenchmarkParse("123456789.120000000")
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
				var err error
				sinkDecimal, err = value.Rescale(tc.scale)
				if err != nil {
					b.Fatal(err)
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
