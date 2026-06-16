package decimal

import "math/bits"

// Add returns d + other. The second result reports whether the result is
// representable. If the exact result needs more mantissa digits than Decimal
// can hold, Add rounds half away from zero while reducing scale.
func (d Decimal) Add(other Decimal) (Decimal, bool) {
	aNeg, aScale, aMant := d.unpack()
	bNeg, bScale, bMant := other.unpack()

	if aMant.isZero() {
		return pack(bNeg, bScale, bMant), true
	}
	if bMant.isZero() {
		return pack(aNeg, aScale, aMant), true
	}

	scale := max(aScale, bScale)
	a, ok := scaleUp(aMant, int(scale-aScale))
	if !ok {
		return Decimal{}, false
	}
	b, ok := scaleUp(bMant, int(scale-bScale))
	if !ok {
		return Decimal{}, false
	}

	if aNeg == bNeg {
		sum, ok := a.add(b)
		if !ok {
			return Decimal{}, false
		}
		return roundAndPack(aNeg, int(scale), sum)
	}

	switch a.cmp(b) {
	case 0:
		return Decimal{}, true
	case 1:
		diff, _ := a.sub(b)
		return roundAndPack(aNeg, int(scale), diff)
	default:
		diff, _ := b.sub(a)
		return roundAndPack(bNeg, int(scale), diff)
	}
}

// Sub returns d - other. The second result reports whether the result is
// representable. If the exact result needs more mantissa digits than Decimal
// can hold, Sub rounds half away from zero while reducing scale.
func (d Decimal) Sub(other Decimal) (Decimal, bool) {
	return d.Add(other.Neg())
}

// Mul returns d * other. The second result reports whether the result is
// representable. If the exact result has more than MaxScale fractional digits
// or needs more mantissa digits than Decimal can hold, Mul rounds half away
// from zero while reducing scale.
func (d Decimal) Mul(other Decimal) (Decimal, bool) {
	aNeg, aScale, aMant := d.unpack()
	bNeg, bScale, bMant := other.unpack()
	if aMant.isZero() || bMant.isZero() {
		return Decimal{}, true
	}

	return roundAndPack(aNeg != bNeg, int(aScale)+int(bScale), fullMul96(aMant, bMant))
}

// Div returns d / other rounded half away from zero with at most MaxScale
// fractional digits. The second result reports whether the division succeeded
// and the result is representable.
func (d Decimal) Div(other Decimal) (Decimal, bool) {
	aNeg, aScale, aMant := d.unpack()
	bNeg, bScale, bMant := other.unpack()
	if bMant.isZero() {
		return Decimal{}, false
	}
	if aMant.isZero() {
		return Decimal{}, true
	}
	if scale, ok := powerOfTenDivisorScale(bMant, bScale); ok {
		neg := aNeg != bNeg
		if scale >= aScale {
			mantissa, ok := mulPow10Mantissa(aMant, scale-aScale)
			if !ok {
				return Decimal{}, false
			}
			return pack(neg, 0, mantissa), true
		}
		return pack(neg, aScale-scale, aMant).Normalize(), true
	}

	minScale := 0
	if aScale > bScale {
		minScale = int(aScale - bScale)
	}
	neg := aNeg != bNeg
	maxPower := int(bScale) + int(MaxScale) - int(aScale)
	if maxPower <= len(pow10)-1 || bMant.hi != 0 {
		return divByScaledDivisor(aMant, bMant, aScale, bScale, minScale, neg)
	}
	return divBySmallDivisorLargeScale(aMant, bMant.lo, aScale, bScale, minScale, neg)
}

func divByScaledDivisor(aMant, bMant Mantissa, aScale, bScale uint8, minScale int, negative bool) (Decimal, bool) {
	for scale := int(MaxScale); scale >= minScale; scale-- {
		power := int(bScale) + scale - int(aScale)
		if power < 0 {
			continue
		}
		num, ok := scaleUp(aMant, power)
		if !ok {
			// A quotient that fits in Decimal implies
			// scaledNumerator < (maxMantissa+1)*maxMantissa < 2^192.
			continue
		}
		if !quotientFitsMantissa(num, bMant) {
			continue
		}
		var q uint192
		var exact bool
		if bMant.hi == 0 {
			var rem uint64
			q, rem = num.divModSmall(bMant.lo)
			if roundSmallDivisorQuotient(rem, bMant.lo) {
				var ok bool
				q, ok = q.add(uint192{lo: oneMantissa.lo})
				if !ok {
					continue
				}
			}
			exact = rem == 0
		} else {
			var rem Mantissa
			q, rem = divMod192By96(num, bMant)
			if roundDivQuotient(rem, bMant) {
				var ok bool
				q, ok = q.add(uint192{lo: oneMantissa.lo})
				if !ok {
					continue
				}
			}
			exact = rem.isZero()
		}
		mantissa, fits := q.mantissa()
		if !fits {
			continue
		}
		res := pack(negative, uint8FromScale(scale), mantissa)
		if exact {
			res = res.Normalize()
		}
		return res, true
	}
	return Decimal{}, false
}

func divBySmallDivisorLargeScale(aMant Mantissa, divisor uint64, aScale, bScale uint8, minScale int, negative bool) (Decimal, bool) {
	var num uint192
	numPower := 0
	haveNum := false
	for scale := int(MaxScale); scale >= minScale; scale-- {
		power := int(bScale) + scale - int(aScale)
		if power < 0 {
			continue
		}
		if haveNum && power == numPower-1 && power >= len(pow10) {
			num, _ = num.divModSmall(10)
			numPower = power
		} else {
			var ok bool
			num, ok = scaleUp(aMant, power)
			if !ok {
				// A quotient that fits in Decimal implies
				// scaledNumerator < (maxMantissa+1)*maxMantissa < 2^192.
				haveNum = false
				continue
			}
			haveNum = true
			numPower = power
		}
		if !quotientFitsSmallMantissa(num, divisor) {
			continue
		}
		if res, ok := divScaledSmall(num, divisor, negative, scale); ok {
			return res, true
		}
	}
	return Decimal{}, false
}

// Remainder returns d % other. The result has the sign of d and the common
// operand scale, matching .NET decimal remainder behavior.
func (d Decimal) Remainder(other Decimal) (Decimal, bool) {
	aNeg, aScale, aMant := d.unpack()
	_, bScale, bMant := other.unpack()
	if bMant.isZero() {
		return Decimal{}, false
	}
	if aMant.isZero() {
		return Decimal{}, true
	}

	scale := max(aScale, bScale)
	numerator, ok := scaleUp(aMant, int(scale-aScale))
	if !ok {
		return Decimal{}, false
	}

	var divisor Mantissa
	if scale == bScale {
		divisor = bMant
	} else {
		divisor, ok = mulPow10Mantissa(bMant, scale-bScale)
		if !ok {
			return pack(aNeg, scale, aMant), true
		}
	}

	var remainder Mantissa
	if divisor.hi == 0 {
		var r uint64
		_, r = numerator.divModSmall(divisor.lo)
		remainder = MantissaFromUint64(r)
	} else {
		_, remainder = divMod192By96(numerator, divisor)
	}
	return pack(aNeg, scale, remainder), true
}

func divScaledSmall(num uint192, divisor uint64, negative bool, scale int) (Decimal, bool) {
	q, rem := num.divModSmall(divisor)
	if roundSmallDivisorQuotient(rem, divisor) {
		var ok bool
		q, ok = q.add(uint192{lo: oneMantissa.lo})
		if !ok {
			return Decimal{}, false
		}
	}
	mantissa, fits := q.mantissa()
	if !fits {
		return Decimal{}, false
	}
	res := pack(negative, uint8FromScale(scale), mantissa)
	if rem == 0 {
		res = res.Normalize()
	}
	return res, true
}

// roundSmallDivisorQuotient compares 2*remainder >= divisor without overflowing.
func roundSmallDivisorQuotient(remainder, divisor uint64) bool {
	return remainder >= divisor/2+divisor%2
}

func quotientFitsMantissa(n uint192, divisor Mantissa) bool {
	thresholdHi := uint64(divisor.hi)<<32 | divisor.lo>>32
	if n.hi != thresholdHi {
		return n.hi < thresholdHi
	}
	return n.mid < divisor.lo<<32
}

func quotientFitsSmallMantissa(n uint192, divisor uint64) bool {
	thresholdHi := divisor >> 32
	if n.hi != thresholdHi {
		return n.hi < thresholdHi
	}
	return n.mid < divisor<<32
}

func powerOfTenDivisorScale(m Mantissa, scale uint8) (uint8, bool) {
	if m == oneMantissa {
		return scale, true
	}
	if scale == 0 || m.lo&1 != 0 || (m.lo%5+uint64(m.hi)%5)%5 != 0 {
		return 0, false
	}
	for scale > 0 {
		q, r := m.divMod64(10)
		if r != 0 {
			return 0, false
		}
		m = q
		scale--
		if m == oneMantissa {
			return scale, true
		}
	}
	return 0, false
}

func roundAndPack(negative bool, scale int, n uint192) (Decimal, bool) {
	if n.isZero() {
		return Decimal{}, true
	}
	drop := max(0, scale-int(MaxScale))
	q := n
	var rem uint64
	if drop > 0 {
		q, rem = divPow10WithRoundingDigit(n, drop)
	}
	for ; drop <= scale; drop++ {
		rounded := q
		if drop > 0 && rem >= 5 {
			rounded, _ = rounded.add(uint192{lo: oneMantissa.lo})
		}
		if mantissa, fits := rounded.mantissa(); fits {
			return pack(negative, uint8FromScale(scale-drop), mantissa), true
		}
		if drop == scale {
			break
		}
		q, rem = q.divModSmall(10)
	}
	return Decimal{}, false
}

// divPow10WithRoundingDigit returns floor(n/10^drop) and the first dropped digit.
func divPow10WithRoundingDigit(n uint192, drop int) (uint192, uint64) {
	if drop == 0 {
		return n, 0
	}
	if drop == int(MaxScale) {
		q, r := divMod192By96(n, pow10[MaxScale])
		return q, firstDroppedDigitAtMaxScale(r)
	}

	q := n
	for drop > 1 {
		step := min(drop-1, 19)
		q, _ = q.divModSmall(pow10[step].lo)
		drop -= step
	}
	return q.divModSmall(10)
}

// pow10To27Multiples[d] is d*10^27, used to extract the first dropped digit
// after dividing by 10^28 without another wide division.
var pow10To27Multiples = [...]Mantissa{
	NewMantissa(0x0, 0x0),
	NewMantissa(0x33b2e3c, 0x9fd0803ce8000000),
	NewMantissa(0x6765c79, 0x3fa10079d0000000),
	NewMantissa(0x9b18ab5, 0xdf7180b6b8000000),
	NewMantissa(0xcecb8f2, 0x7f4200f3a0000000),
	NewMantissa(0x1027e72f, 0x1f12813088000000),
	NewMantissa(0x1363156b, 0xbee3016d70000000),
	NewMantissa(0x169e43a8, 0x5eb381aa58000000),
	NewMantissa(0x19d971e4, 0xfe8401e740000000),
	NewMantissa(0x1d14a021, 0x9e54822428000000),
}

func firstDroppedDigitAtMaxScale(remainder Mantissa) uint64 {
	for digit := 9; digit > 0; digit-- {
		if remainder.cmp(pow10To27Multiples[digit]) >= 0 {
			return uint64(digit)
		}
	}
	return 0
}

func scaleUp(m Mantissa, scale int) (uint192, bool) {
	if scale == 0 {
		return uint192{mid: uint64(m.hi), lo: m.lo}, true
	}
	if scale <= 19 {
		return fullMul96By64(m, pow10[scale].lo), true
	}

	step := min(scale, len(pow10)-1)
	res := fullMul96(m, pow10[step])
	scale -= step
	for scale > 0 {
		step = min(scale, len(pow10)-1)
		var ok bool
		if step <= 19 {
			res, ok = res.mul64(pow10[step].lo)
		} else {
			res, ok = res.mul96(pow10[step])
		}
		if !ok {
			return uint192{}, false
		}
		scale -= step
	}
	return res, true
}

func fullMul96By64(a Mantissa, b uint64) uint192 {
	llHi, llLo := bits.Mul64(a.lo, b)
	hlHi, hlLo := bits.Mul64(uint64(a.hi), b)
	mid, carry := bits.Add64(llHi, hlLo, 0)
	return uint192{hi: hlHi + carry, mid: mid, lo: llLo}
}

func fullMul96(a, b Mantissa) uint192 {
	llHi, llLo := bits.Mul64(a.lo, b.lo)
	lhHi, lhLo := bits.Mul64(a.lo, uint64(b.hi))
	hlHi, hlLo := bits.Mul64(uint64(a.hi), b.lo)
	hh := uint64(a.hi) * uint64(b.hi)

	mid := llHi
	hi := hh
	var carry uint64

	mid, carry = bits.Add64(mid, lhLo, 0)
	hi += carry
	mid, carry = bits.Add64(mid, hlLo, 0)
	hi += carry
	hi, _ = bits.Add64(hi, lhHi, 0)
	hi, _ = bits.Add64(hi, hlHi, 0)

	return uint192{hi: hi, mid: mid, lo: llLo}
}

// divMod192By96 divides a 192-bit numerator by a 96-bit divisor using Knuth's
// normalized long division. Limbs are little-endian 64-bit words in u; v1 is the
// divisor's high 32 bits widened to a word. Each quotient estimate is corrected
// at most once after subtracting qhat*v from the active three-word window.
func divMod192By96(n uint192, d Mantissa) (uint192, Mantissa) {
	if d.hi == 0 {
		q, r := n.divModSmall(d.lo)
		return q, MantissaFromUint64(r)
	}

	u := [4]uint64{n.lo, n.mid, n.hi}
	v0, v1 := d.lo, uint64(d.hi)
	shift := bits.LeadingZeros64(v1)
	if shift > 0 {
		u[3] = u[2] >> (64 - shift)
		u[2] = u[2]<<shift | u[1]>>(64-shift)
		u[1] = u[1]<<shift | u[0]>>(64-shift)
		u[0] <<= shift
		v1 = v1<<shift | v0>>(64-shift)
		v0 <<= shift
	}

	var q [2]uint64
	for j := 1; j >= 0; j-- {
		qhat := divEstimate(u[j+2], u[j+1], u[j], v1, v0)
		p2, p1, p0 := mul128By64(v1, v0, qhat)

		var borrow uint64
		u[j], borrow = bits.Sub64(u[j], p0, 0)
		u[j+1], borrow = bits.Sub64(u[j+1], p1, borrow)
		u[j+2], borrow = bits.Sub64(u[j+2], p2, borrow)
		if borrow != 0 {
			qhat--
			var carry uint64
			u[j], carry = bits.Add64(u[j], v0, 0)
			u[j+1], carry = bits.Add64(u[j+1], v1, carry)
			u[j+2] += carry
		}
		q[j] = qhat
	}

	remHi, remLo := u[1], u[0]
	if shift > 0 {
		remHi = u[1] >> shift
		remLo = u[0]>>shift | u[1]<<(64-shift)
	}
	return uint192{mid: q[1], lo: q[0]}, Mantissa{hi: uint32FromChecked(remHi), lo: remLo}
}

func divEstimate(u2, u1, u0, v1, v0 uint64) uint64 {
	var qhat, rhat uint64
	if u2 >= v1 {
		qhat = ^uint64(0)
		var carry uint64
		rhat, carry = bits.Add64(u1, v1, 0)
		if carry != 0 {
			return qhat
		}
	} else {
		qhat, rhat = bits.Div64(u2, u1, v1)
	}

	for {
		hi, lo := bits.Mul64(qhat, v0)
		if hi < rhat || hi == rhat && lo <= u0 {
			return qhat
		}
		qhat--
		var carry uint64
		rhat, carry = bits.Add64(rhat, v1, 0)
		if carry != 0 {
			return qhat
		}
	}
}

func mul128By64(hi, lo, x uint64) (uint64, uint64, uint64) {
	hi0, lo0 := bits.Mul64(x, lo)
	hi1, lo1 := bits.Mul64(x, hi)
	mid, carry := bits.Add64(hi0, lo1, 0)
	return hi1 + carry, mid, lo0
}

func roundDivQuotient(remainder Mantissa, divisor Mantissa) bool {
	if remainder.isZero() {
		return false
	}
	double, overflow := remainder.addChecked(remainder)
	return overflow || double.cmp(divisor) >= 0
}

// uint192 is the private scratch width for decimal arithmetic. Decimal stores
// a 96-bit mantissa; multiplying two mantissas can need 192 bits before
// rounding, and division scales the numerator before a 96-bit divisor divide.
type uint192 struct {
	hi  uint64
	mid uint64
	lo  uint64
}

func (n uint192) isZero() bool {
	return n.hi == 0 && n.mid == 0 && n.lo == 0
}

func (n uint192) cmp(other uint192) int {
	if n.hi < other.hi {
		return -1
	}
	if n.hi > other.hi {
		return 1
	}
	if n.mid < other.mid {
		return -1
	}
	if n.mid > other.mid {
		return 1
	}
	if n.lo < other.lo {
		return -1
	}
	if n.lo > other.lo {
		return 1
	}
	return 0
}

func (n uint192) mantissa() (Mantissa, bool) {
	if n.hi != 0 || n.mid > maxMantissaHiWord {
		return Mantissa{}, false
	}
	return Mantissa{hi: uint32FromChecked(n.mid), lo: n.lo}, true
}

func uint8FromScale(scale int) uint8 {
	if scale < 0 || scale > int(MaxScale) {
		panic("decimal: internal scale overflow")
	}
	return uint8(scale)
}

func (n uint192) add(other uint192) (uint192, bool) {
	lo, carry := bits.Add64(n.lo, other.lo, 0)
	mid, carry := bits.Add64(n.mid, other.mid, carry)
	hi, carry := bits.Add64(n.hi, other.hi, carry)
	return uint192{hi: hi, mid: mid, lo: lo}, carry == 0
}

func (n uint192) sub(other uint192) (uint192, bool) {
	lo, borrow := bits.Sub64(n.lo, other.lo, 0)
	mid, borrow := bits.Sub64(n.mid, other.mid, borrow)
	hi, borrow := bits.Sub64(n.hi, other.hi, borrow)
	return uint192{hi: hi, mid: mid, lo: lo}, borrow == 0
}

func (n uint192) mul96(m Mantissa) (uint192, bool) {
	var out [6]uint64
	overflow := addMul64Into(&out[0], &out[1], &out[2], &out[3], n.lo, m.lo)
	overflow = addMul64Into(&out[1], &out[2], &out[3], &out[4], n.lo, uint64(m.hi)) || overflow
	overflow = addMul64Into(&out[1], &out[2], &out[3], &out[4], n.mid, m.lo) || overflow
	overflow = addMul64Into(&out[2], &out[3], &out[4], &out[5], n.mid, uint64(m.hi)) || overflow
	overflow = addMul64Into(&out[2], &out[3], &out[4], &out[5], n.hi, m.lo) || overflow
	var spill uint64
	overflow = addMul64Into(&out[3], &out[4], &out[5], &spill, n.hi, uint64(m.hi)) || overflow
	if out[3] != 0 || out[4] != 0 || out[5] != 0 {
		return uint192{}, false
	}
	return uint192{hi: out[2], mid: out[1], lo: out[0]}, !overflow
}

func (n uint192) mul64(m uint64) (uint192, bool) {
	hi0, lo := bits.Mul64(n.lo, m)
	hi1, lo1 := bits.Mul64(n.mid, m)
	hi2, lo2 := bits.Mul64(n.hi, m)

	mid, carry := bits.Add64(hi0, lo1, 0)
	hi, carry := bits.Add64(hi1, lo2, carry)
	return uint192{hi: hi, mid: mid, lo: lo}, hi2 == 0 && carry == 0
}

func addMul64Into(w0, w1, w2, w3 *uint64, a, b uint64) bool {
	if a == 0 || b == 0 {
		return false
	}
	hi, lo := bits.Mul64(a, b)

	var carry uint64
	*w0, carry = bits.Add64(*w0, lo, 0)
	hi, carryHi := bits.Add64(hi, 0, carry)
	*w1, carry = bits.Add64(*w1, hi, 0)
	carry |= carryHi
	if carry == 0 {
		return false
	}
	*w2, carry = bits.Add64(*w2, 0, carry)
	if carry == 0 {
		return false
	}
	*w3, carry = bits.Add64(*w3, 0, carry)
	return carry != 0
}

func (n uint192) divModSmall(divisor uint64) (uint192, uint64) {
	if divisor == 10 {
		return n.divMod10()
	}
	if n.hi == 0 {
		if n.mid == 0 {
			return uint192{lo: n.lo / divisor}, n.lo % divisor
		}
		var rem uint64
		mid, rem := bits.Div64(0, n.mid, divisor)
		lo, rem := bits.Div64(rem, n.lo, divisor)
		return uint192{mid: mid, lo: lo}, rem
	}
	var rem uint64
	hi, rem := bits.Div64(0, n.hi, divisor)
	mid, rem := bits.Div64(rem, n.mid, divisor)
	lo, rem := bits.Div64(rem, n.lo, divisor)
	return uint192{hi: hi, mid: mid, lo: lo}, rem
}

func (n uint192) divMod10() (uint192, uint64) {
	if n.hi == 0 {
		if n.mid == 0 {
			return uint192{lo: n.lo / 10}, n.lo % 10
		}
		mid, rem := n.mid/10, n.mid%10
		lo, rem := div128By10(rem, n.lo)
		return uint192{mid: mid, lo: lo}, rem
	}
	hi, rem := n.hi/10, n.hi%10
	mid, rem := div128By10(rem, n.mid)
	lo, rem := div128By10(rem, n.lo)
	return uint192{hi: hi, mid: mid, lo: lo}, rem
}
