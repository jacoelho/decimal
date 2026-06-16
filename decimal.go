package decimal

import (
	"errors"
	"strconv"
)

// Decimal is a signed fixed-size decimal.
//
// The representation follows .NET decimal: a 96-bit unsigned mantissa plus a
// 32-bit flags word carrying sign and scale. The value is mantissa * 10^-scale,
// negated when the sign bit is set. Scale is the number of fractional decimal
// digits and is always in [0, 28]. The zero value is the canonical zero.
// Decimal values are comparable, but == compares the stored representation; use
// Cmp or Equal for numeric equality.
//
//nolint:recvcheck // Decode and SQL scan APIs must mutate through *Decimal; value receivers keep Decimal immutable elsewhere.
type Decimal struct {
	lo    uint64
	hi    uint32
	flags uint32
}

const (
	// MaxScale is the largest supported number of fractional decimal digits.
	MaxScale uint8 = 28

	maxMantissaDecimal = "79228162514264337593543950335"
	decimalDigitPairs  = "0001020304050607080910111213141516171819" +
		"2021222324252627282930313233343536373839" +
		"4041424344454647484950515253545556575859" +
		"6061626364656667686970717273747576777879" +
		"8081828384858687888990919293949596979899"

	dotnetScaleShift  = 16
	dotnetScaleMask   = uint32(0xff) << dotnetScaleShift
	dotnetSignMask    = uint32(1) << 31
	dotnetReserved    = uint32(0x7f00ffff)
	maxMantissaHiWord = uint64(^uint32(0))
)

// RoundingMode selects how discarded fractional digits are handled by Round.
type RoundingMode uint8

const (
	// RoundHalfAwayFromZero rounds halfway cases away from zero.
	RoundHalfAwayFromZero RoundingMode = iota
	// RoundHalfToEven rounds halfway cases to the nearest even final digit.
	RoundHalfToEven
	// RoundTowardZero truncates discarded fractional digits.
	RoundTowardZero
	// RoundTowardNegativeInfinity rounds toward negative infinity.
	RoundTowardNegativeInfinity
	// RoundTowardPositiveInfinity rounds toward positive infinity.
	RoundTowardPositiveInfinity
)

var (
	// ErrInvalid reports invalid decimal parts or input.
	ErrInvalid = errors.New("decimal: invalid")
	// ErrOverflow reports a value that cannot fit in Decimal.
	ErrOverflow = errors.New("decimal: overflow")

	maxMantissa = NewMantissa(^uint32(0), ^uint64(0))
	oneMantissa = MantissaFromUint64(1)
	tenMantissa = MantissaFromUint64(10)
	pow10       = [...]Mantissa{
		NewMantissa(0x0, 0x1),
		NewMantissa(0x0, 0xa),
		NewMantissa(0x0, 0x64),
		NewMantissa(0x0, 0x3e8),
		NewMantissa(0x0, 0x2710),
		NewMantissa(0x0, 0x186a0),
		NewMantissa(0x0, 0xf4240),
		NewMantissa(0x0, 0x989680),
		NewMantissa(0x0, 0x5f5e100),
		NewMantissa(0x0, 0x3b9aca00),
		NewMantissa(0x0, 0x2540be400),
		NewMantissa(0x0, 0x174876e800),
		NewMantissa(0x0, 0xe8d4a51000),
		NewMantissa(0x0, 0x9184e72a000),
		NewMantissa(0x0, 0x5af3107a4000),
		NewMantissa(0x0, 0x38d7ea4c68000),
		NewMantissa(0x0, 0x2386f26fc10000),
		NewMantissa(0x0, 0x16345785d8a0000),
		NewMantissa(0x0, 0xde0b6b3a7640000),
		NewMantissa(0x0, 0x8ac7230489e80000),
		NewMantissa(0x5, 0x6bc75e2d63100000),
		NewMantissa(0x36, 0x35c9adc5dea00000),
		NewMantissa(0x21e, 0x19e0c9bab2400000),
		NewMantissa(0x152d, 0x2c7e14af6800000),
		NewMantissa(0xd3c2, 0x1bcecceda1000000),
		NewMantissa(0x84595, 0x161401484a000000),
		NewMantissa(0x52b7d2, 0xdcc80cd2e4000000),
		NewMantissa(0x33b2e3c, 0x9fd0803ce8000000),
		NewMantissa(0x204fce5e, 0x3e25026110000000),
	}
)

// MaxMantissa returns the largest representable unsigned mantissa.
func MaxMantissa() Mantissa {
	return maxMantissa
}

// Max returns the largest positive Decimal value.
func Max() Decimal {
	return pack(false, 0, maxMantissa)
}

// Min returns the smallest negative Decimal value.
func Min() Decimal {
	return pack(true, 0, maxMantissa)
}

// New constructs a Decimal from mantissa, scale, and sign.
func New(mantissa Mantissa, scale uint8, negative bool) (Decimal, error) {
	d, ok := fromParts(mantissa, scale, negative)
	if !ok {
		return Decimal{}, ErrInvalid
	}
	return d, nil
}

// MustNew constructs a Decimal and panics if the parts are invalid.
func MustNew(mantissa Mantissa, scale uint8, negative bool) Decimal {
	d, err := New(mantissa, scale, negative)
	if err != nil {
		panic(err)
	}
	return d
}

func fromParts(mantissa Mantissa, scale uint8, negative bool) (Decimal, bool) {
	if scale > MaxScale {
		return Decimal{}, false
	}
	return pack(negative, scale, mantissa), true
}

// FromBits constructs a Decimal from .NET decimal words.
func FromBits(lo, mid, hi, flags uint32) (Decimal, error) {
	scale := uint8((flags & dotnetScaleMask) >> dotnetScaleShift)
	if flags&dotnetReserved != 0 || scale > MaxScale {
		return Decimal{}, ErrInvalid
	}
	mantissa := NewMantissa(hi, uint64(mid)<<32|uint64(lo))
	return pack(flags&dotnetSignMask != 0, scale, mantissa), nil
}

// MustFromBits constructs a Decimal from .NET decimal words and panics if they
// are invalid.
func MustFromBits(lo, mid, hi, flags uint32) Decimal {
	d, err := FromBits(lo, mid, hi, flags)
	if err != nil {
		panic(err)
	}
	return d
}

// Bits returns the .NET decimal words: low 32 mantissa bits, middle 32
// mantissa bits, high 32 mantissa bits, and flags.
func (d Decimal) Bits() (lo, mid, hi, flags uint32) {
	if d.isZero() {
		return 0, 0, 0, 0
	}
	lo, mid = splitUint64(d.lo)
	return lo, mid, d.hi, d.flags
}

// FromUint64 constructs a Decimal from an unsigned integer.
func FromUint64(v uint64) Decimal {
	return pack(false, 0, MantissaFromUint64(v))
}

// FromInt64 constructs a Decimal from a signed integer.
func FromInt64(v int64) Decimal {
	if v >= 0 {
		return FromUint64(uint64(v))
	}
	mag := uint64(-(v + 1)) + 1 //nolint:gosec // v is negative; v+1 avoids math.MinInt64 overflow.
	return pack(true, 0, MantissaFromUint64(mag))
}

// Parts returns mantissa, scale, and sign.
func (d Decimal) Parts() (Mantissa, uint8, bool) {
	mantissa := d.Mantissa()
	if mantissa.isZero() {
		return Mantissa{}, 0, false
	}
	return mantissa, d.scale(), d.flags&dotnetSignMask != 0
}

// Mantissa returns the unsigned mantissa.
func (d Decimal) Mantissa() Mantissa {
	return NewMantissa(d.hi, d.lo)
}

// Scale returns the number of fractional decimal digits.
func (d Decimal) Scale() uint8 {
	if d.isZero() {
		return 0
	}
	return d.scale()
}

// IsNegative reports whether d is less than zero.
func (d Decimal) IsNegative() bool {
	return d.flags&dotnetSignMask != 0 && !d.isZero()
}

// IsPositive reports whether d is greater than zero.
func (d Decimal) IsPositive() bool {
	return d.flags&dotnetSignMask == 0 && !d.isZero()
}

// IsZero reports whether d is zero.
func (d Decimal) IsZero() bool {
	return d.isZero()
}

// Sign returns -1 for negative values, 0 for zero, and +1 for positive values.
func (d Decimal) Sign() int {
	if d.isZero() {
		return 0
	}
	if d.flags&dotnetSignMask != 0 {
		return -1
	}
	return 1
}

// Neg returns -d.
func (d Decimal) Neg() Decimal {
	if d.isZero() {
		return Decimal{}
	}
	d.flags ^= dotnetSignMask
	return d
}

// Abs returns |d|.
func (d Decimal) Abs() Decimal {
	d.flags &^= dotnetSignMask
	return d
}

// Normalize removes trailing fractional zeroes.
func (d Decimal) Normalize() Decimal {
	neg, scale, mantissa := d.unpack()
	if mantissa.isZero() {
		return Decimal{}
	}
	for scale > 0 {
		q, r := mantissa.divMod64(10)
		if r != 0 {
			break
		}
		mantissa = q
		scale--
	}
	return pack(neg, scale, mantissa)
}

// Rescale returns d with a new scale. Increasing scale is exact and can
// overflow. Decreasing scale rounds half away from zero.
func (d Decimal) Rescale(scale uint8) (Decimal, bool) {
	if scale > MaxScale {
		return Decimal{}, false
	}
	neg, curScale, mantissa := d.unpack()
	if mantissa.isZero() || scale == curScale {
		return pack(neg, scale, mantissa), true
	}
	if scale > curScale {
		up, ok := mulPow10Mantissa(mantissa, scale-curScale)
		if !ok {
			return Decimal{}, false
		}
		return pack(neg, scale, up), true
	}
	return pack(neg, scale, divRoundPow10(mantissa, curScale-scale)), true
}

// Round returns d rounded to at most scale fractional digits using mode.
func (d Decimal) Round(scale uint8, mode RoundingMode) (Decimal, bool) {
	if scale > MaxScale || !validRoundingMode(mode) {
		return Decimal{}, false
	}

	neg, curScale, mantissa := d.unpack()
	if mantissa.isZero() || scale >= curScale {
		return pack(neg, curScale, mantissa), true
	}

	quotient, remainder := divModPow10Mantissa(mantissa, curScale-scale)
	if shouldRoundUp(mode, neg, quotient, remainder, curScale-scale) {
		rounded, overflow := quotient.addChecked(oneMantissa)
		if overflow {
			return Decimal{}, false
		}
		quotient = rounded
	}
	return pack(neg, scale, quotient), true
}

// RoundTo returns d rounded to at most scale fractional digits.
func (d Decimal) RoundTo(scale uint8) Decimal {
	if scale >= d.Scale() {
		return d
	}
	res, _ := d.Round(scale, RoundHalfAwayFromZero)
	return res
}

// Truncate returns the integral part of d, rounded toward zero.
func (d Decimal) Truncate() Decimal {
	neg, scale, mantissa := d.unpack()
	if scale == 0 || mantissa.isZero() {
		return pack(neg, scale, mantissa)
	}
	quotient, _ := divModPow10Mantissa(mantissa, scale)
	return pack(neg, 0, quotient)
}

// Floor returns the greatest integral Decimal less than or equal to d.
func (d Decimal) Floor() Decimal {
	neg, scale, mantissa := d.unpack()
	if scale == 0 || mantissa.isZero() {
		return pack(neg, scale, mantissa)
	}
	quotient, remainder := divModPow10Mantissa(mantissa, scale)
	if neg && !remainder.isZero() {
		rounded, overflow := quotient.addChecked(oneMantissa)
		if overflow {
			panic("decimal: internal floor overflow")
		}
		quotient = rounded
	}
	return pack(neg, 0, quotient)
}

// Ceiling returns the smallest integral Decimal greater than or equal to d.
func (d Decimal) Ceiling() Decimal {
	neg, scale, mantissa := d.unpack()
	if scale == 0 || mantissa.isZero() {
		return pack(neg, scale, mantissa)
	}
	quotient, remainder := divModPow10Mantissa(mantissa, scale)
	if !neg && !remainder.isZero() {
		rounded, overflow := quotient.addChecked(oneMantissa)
		if overflow {
			panic("decimal: internal ceiling overflow")
		}
		quotient = rounded
	}
	return pack(neg, 0, quotient)
}

// Cmp compares d and other.
func (d Decimal) Cmp(other Decimal) int {
	aNeg, aScale, aMant := d.unpack()
	bNeg, bScale, bMant := other.unpack()

	if aMant.isZero() {
		if bMant.isZero() {
			return 0
		}
		if bNeg {
			return 1
		}
		return -1
	}
	if bMant.isZero() {
		if aNeg {
			return -1
		}
		return 1
	}
	if aNeg != bNeg {
		if aNeg {
			return -1
		}
		return 1
	}

	cmp := cmpAbs(aMant, aScale, bMant, bScale)
	if aNeg {
		return -cmp
	}
	return cmp
}

// Equal reports whether d and other have the same numeric value. Use Equal or
// Cmp instead of == when scale-insensitive numeric equality is required.
func (d Decimal) Equal(other Decimal) bool {
	return d.Cmp(other) == 0
}

// Min returns the smaller of d and other.
func (d Decimal) Min(other Decimal) Decimal {
	if d.Cmp(other) <= 0 {
		return d
	}
	return other
}

// Max returns the larger of d and other.
func (d Decimal) Max(other Decimal) Decimal {
	if d.Cmp(other) >= 0 {
		return d
	}
	return other
}

// Clamp returns d constrained to [minimum, maximum]. The second result is false
// when minimum is greater than maximum.
func (d Decimal) Clamp(minimum, maximum Decimal) (Decimal, bool) {
	if minimum.Cmp(maximum) > 0 {
		return Decimal{}, false
	}
	if d.Cmp(minimum) < 0 {
		return minimum, true
	}
	if d.Cmp(maximum) > 0 {
		return maximum, true
	}
	return d, true
}

// IsInteger reports whether d has no non-zero fractional digits.
func (d Decimal) IsInteger() bool {
	_, scale, mantissa := d.unpack()
	if scale == 0 || mantissa.isZero() {
		return true
	}
	_, remainder := divModPow10Mantissa(mantissa, scale)
	return remainder.isZero()
}

// Int64 returns d truncated toward zero as int64.
func (d Decimal) Int64() (int64, bool) {
	neg, _, _ := d.unpack()
	mantissa := d.truncatedMantissa()
	if mantissa.hi != 0 {
		return 0, false
	}

	const (
		maxInt64          = uint64(1<<63 - 1)
		minInt64Magnitude = maxInt64 + 1
	)
	if neg {
		if mantissa.lo > minInt64Magnitude {
			return 0, false
		}
		if mantissa.lo == minInt64Magnitude {
			return -1 << 63, true
		}
		return -int64(mantissa.lo), true
	}
	if mantissa.lo > maxInt64 {
		return 0, false
	}
	return int64(mantissa.lo), true
}

// Uint64 returns d truncated toward zero as uint64.
func (d Decimal) Uint64() (uint64, bool) {
	neg, _, _ := d.unpack()
	mantissa := d.truncatedMantissa()
	if mantissa.hi != 0 || neg && !mantissa.isZero() {
		return 0, false
	}
	return mantissa.lo, true
}

// Int32 returns d truncated toward zero as int32.
func (d Decimal) Int32() (int32, bool) {
	v, ok := d.Int64()
	if !ok || v < -1<<31 || v > 1<<31-1 {
		return 0, false
	}
	return int32(v), true
}

// Uint32 returns d truncated toward zero as uint32.
func (d Decimal) Uint32() (uint32, bool) {
	v, ok := d.Uint64()
	if !ok || v > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(v), true
}

// String returns the decimal string form, preserving scale for non-zero values.
func (d Decimal) String() string {
	var buf [64]byte
	return string(d.appendText(buf[:0]))
}

// AppendText appends the decimal string form to dst.
func (d Decimal) AppendText(dst []byte) ([]byte, error) {
	return d.appendText(dst), nil
}

func (d Decimal) appendText(dst []byte) []byte {
	neg, scale, mantissa := d.unpack()
	if mantissa.isZero() {
		return append(dst, '0')
	}
	if neg {
		dst = append(dst, '-')
	}

	if mantissa.hi == 0 {
		var digits [20]byte
		return appendScaledDigits(dst, strconv.AppendUint(digits[:0], mantissa.lo, 10), scale)
	}

	var digits [29]byte
	return appendScaledDigits(dst, appendMantissaDigits(digits[:0], mantissa), scale)
}

func appendMantissaDigits(dst []byte, m Mantissa) []byte {
	if m.hi == 0 {
		return strconv.AppendUint(dst, m.lo, 10)
	}

	var chunks [4]uint32
	i := len(chunks)
	for !m.isZero() {
		q, r := m.divMod1e9()
		i--
		chunks[i] = r
		m = q
	}

	dst = strconv.AppendUint(dst, uint64(chunks[i]), 10)
	for i++; i < len(chunks); i++ {
		dst = appendNineDigits(dst, chunks[i])
	}
	return dst
}

func appendNineDigits(dst []byte, n uint32) []byte {
	leading := n / 100_000_000
	dst = append(dst, byte('0'+leading))
	n -= leading * 100_000_000

	pair := n / 1_000_000
	dst = appendTwoDigits(dst, pair)
	n -= pair * 1_000_000

	pair = n / 10_000
	dst = appendTwoDigits(dst, pair)
	n -= pair * 10_000

	pair = n / 100
	dst = appendTwoDigits(dst, pair)
	n -= pair * 100

	return appendTwoDigits(dst, n)
}

func appendTwoDigits(dst []byte, n uint32) []byte {
	i := n * 2
	return append(dst, decimalDigitPairs[i], decimalDigitPairs[i+1])
}

func appendScaledDigits(dst, digits []byte, scale uint8) []byte {
	if scale == 0 {
		return append(dst, digits...)
	}

	places := int(scale)
	if len(digits) <= places {
		dst = append(dst, '0', '.')
		dst = appendZeros(dst, places-len(digits))
		return append(dst, digits...)
	}

	split := len(digits) - places
	dst = append(dst, digits[:split]...)
	dst = append(dst, '.')
	return append(dst, digits[split:]...)
}

func pack(negative bool, scale uint8, mantissa Mantissa) Decimal {
	if mantissa.isZero() {
		return Decimal{}
	}
	flags := uint32(scale) << dotnetScaleShift
	if negative {
		flags |= dotnetSignMask
	}
	return Decimal{lo: mantissa.lo, hi: mantissa.hi, flags: flags}
}

func (d Decimal) unpack() (negative bool, scale uint8, mantissa Mantissa) {
	mantissa = d.Mantissa()
	if mantissa.isZero() {
		return false, 0, Mantissa{}
	}
	return d.flags&dotnetSignMask != 0, d.scale(), mantissa
}

func appendZeros(dst []byte, n int) []byte {
	for range n {
		dst = append(dst, '0')
	}
	return dst
}

func splitUint64(v uint64) (lo, hi uint32) {
	return uint32FromChecked(v & maxMantissaHiWord), uint32FromChecked(v >> 32)
}

func uint32FromChecked(v uint64) uint32 {
	if v > maxMantissaHiWord {
		panic("decimal: internal uint32 overflow")
	}
	return uint32(v)
}

func cmpAbs(a Mantissa, aScale uint8, b Mantissa, bScale uint8) int {
	if aScale == bScale {
		return a.cmp(b)
	}
	if aScale < bScale {
		scaled, ok := mulPow10Checked(a, bScale-aScale)
		if !ok {
			return 1
		}
		return scaled.cmp(b)
	}
	scaled, ok := mulPow10Checked(b, aScale-bScale)
	if !ok {
		return -1
	}
	return a.cmp(scaled)
}

func mulPow10Mantissa(m Mantissa, scale uint8) (Mantissa, bool) {
	res, ok := mulPow10Checked(m, scale)
	if !ok || res.cmp(maxMantissa) > 0 {
		return Mantissa{}, false
	}
	return res, true
}

func mulPow10Checked(m Mantissa, scale uint8) (Mantissa, bool) {
	if int(scale) >= len(pow10) {
		return Mantissa{}, false
	}
	res, overflow := m.mulChecked(pow10[scale])
	if overflow {
		return Mantissa{}, false
	}
	return res, true
}

func divModPow10Mantissa(m Mantissa, scale uint8) (Mantissa, Mantissa) {
	if scale == 0 || m.isZero() {
		return m, Mantissa{}
	}
	if scale <= 19 {
		quotient, remainder := m.divMod64(pow10[scale].lo)
		return quotient, MantissaFromUint64(remainder)
	}
	quotient, remainder := divMod192By96(uint192{mid: uint64(m.hi), lo: m.lo}, pow10[scale])
	mantissa, ok := quotient.mantissa()
	if !ok {
		panic("decimal: internal pow10 quotient overflow")
	}
	return mantissa, remainder
}

func validRoundingMode(mode RoundingMode) bool {
	return mode <= RoundTowardPositiveInfinity
}

func shouldRoundUp(mode RoundingMode, negative bool, quotient, remainder Mantissa, drop uint8) bool {
	if remainder.isZero() {
		return false
	}
	switch mode {
	case RoundHalfAwayFromZero:
		return remainder.cmp(halfPow10(drop)) >= 0
	case RoundHalfToEven:
		cmp := remainder.cmp(halfPow10(drop))
		return cmp > 0 || cmp == 0 && quotient.lo&1 == 1
	case RoundTowardZero:
		return false
	case RoundTowardNegativeInfinity:
		return negative
	case RoundTowardPositiveInfinity:
		return !negative
	default:
		panic("decimal: invalid rounding mode")
	}
}

func halfPow10(scale uint8) Mantissa {
	half, remainder := pow10[scale].divMod64(2)
	if remainder != 0 {
		panic("decimal: internal half power of ten remainder")
	}
	return half
}

func (d Decimal) truncatedMantissa() Mantissa {
	_, scale, mantissa := d.unpack()
	if scale == 0 || mantissa.isZero() {
		return mantissa
	}
	quotient, _ := divModPow10Mantissa(mantissa, scale)
	return quotient
}

func divRoundPow10(m Mantissa, scale uint8) Mantissa {
	if scale == 0 || m.isZero() {
		return m
	}
	if scale == 1 {
		q, remainder := m.divMod64(10)
		if remainder >= 5 {
			rounded, overflow := q.addChecked(oneMantissa)
			if overflow {
				panic("decimal: internal pow10 rounding overflow")
			}
			q = rounded
		}
		return q
	}
	q, remainder := divModPow10Mantissa(m, scale)
	if remainder.cmp(halfPow10(scale)) >= 0 {
		rounded, overflow := q.addChecked(oneMantissa)
		if overflow {
			panic("decimal: internal pow10 rounding overflow")
		}
		q = rounded
	}
	return q
}

func (d Decimal) isZero() bool {
	return d.hi == 0 && d.lo == 0
}

func (d Decimal) scale() uint8 {
	return uint8((d.flags & dotnetScaleMask) >> dotnetScaleShift)
}
