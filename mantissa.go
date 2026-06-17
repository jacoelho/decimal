package decimal

import "math/bits"

// Mantissa is the unsigned 96-bit coefficient stored by Decimal.
type Mantissa struct {
	lo uint64
	hi uint32
}

// NewMantissa constructs a Mantissa from numeric high and low words. hi
// contains bits 95:64 and lo contains bits 63:0; this is not a byte-order API.
func NewMantissa(hi uint32, lo uint64) Mantissa {
	return Mantissa{hi: hi, lo: lo}
}

// MantissaFromUint64 widens a 64-bit unsigned value to Mantissa.
func MantissaFromUint64(x uint64) Mantissa {
	return Mantissa{lo: x}
}

// Words returns the numeric high and low words of m. hi contains bits 95:64 and
// lo contains bits 63:0; this is not a byte-order API.
func (m Mantissa) Words() (hi uint32, lo uint64) {
	return m.hi, m.lo
}

func (m Mantissa) isZero() bool {
	return m.hi == 0 && m.lo == 0
}

func (m Mantissa) cmp(other Mantissa) int {
	if m.hi < other.hi {
		return -1
	}
	if m.hi > other.hi {
		return 1
	}
	if m.lo < other.lo {
		return -1
	}
	if m.lo > other.lo {
		return 1
	}
	return 0
}

func (m Mantissa) addChecked(other Mantissa) (Mantissa, bool) {
	lo, carryLo := bits.Add64(m.lo, other.lo, 0)
	hi, carryHi := bits.Add64(uint64(m.hi), uint64(other.hi), carryLo)
	overflow := carryHi != 0 || hi > uint64(^uint32(0))
	if overflow {
		return Mantissa{}, true
	}
	return Mantissa{hi: uint32FromChecked(hi), lo: lo}, false
}

func (m Mantissa) mulChecked(other Mantissa) (Mantissa, bool) {
	product := fullMul96(m, other)
	got, ok := product.mantissa()
	return got, !ok
}

func (m Mantissa) mul10Add(digit byte) (Mantissa, bool) {
	carry, lo := bits.Mul64(m.lo, 10)
	lo, addCarry := bits.Add64(lo, uint64(digit), 0)
	hi := uint64(m.hi)*10 + carry + addCarry
	if hi > maxMantissaHiWord {
		return Mantissa{}, true
	}
	return Mantissa{hi: uint32FromChecked(hi), lo: lo}, false
}

func (m Mantissa) divMod64(divisor uint64) (Mantissa, uint64) {
	if m.hi == 0 {
		return Mantissa{lo: m.lo / divisor}, m.lo % divisor
	}
	if divisor == 10 {
		hi := uint64(m.hi)
		qHi, rem := hi/10, hi%10
		qLo, rem := div128By10(rem, m.lo)
		return Mantissa{hi: uint32FromChecked(qHi), lo: qLo}, rem
	}
	var rem uint64
	hi, rem := bits.Div64(0, uint64(m.hi), divisor)
	lo, rem := bits.Div64(rem, m.lo, divisor)
	return Mantissa{hi: uint32FromChecked(hi), lo: lo}, rem
}

func (m Mantissa) divMod1e9() (Mantissa, uint32) {
	const divisor = uint64(1_000_000_000)
	if m.hi == 0 {
		return Mantissa{lo: m.lo / divisor}, uint32FromChecked(m.lo % divisor)
	}
	hi := uint64(m.hi)
	qHi, rem := hi/divisor, hi%divisor
	qLo, rem := bits.Div64(rem, m.lo, divisor)
	return Mantissa{hi: uint32FromChecked(qHi), lo: qLo}, uint32FromChecked(rem)
}

func div128By10(rem, lo uint64) (uint64, uint64) {
	// 2^64 = 10*floor((2^64-1)/10) + ((2^64-1)%10 + 1).
	// rem is the carried remainder from the higher word, so rem < 10.
	const div10BaseQuotient = ^uint64(0) / 10

	q := rem*div10BaseQuotient + lo/10
	extra := rem*(^uint64(0)%10+1) + lo%10
	q += extra / 10
	return q, extra % 10
}
