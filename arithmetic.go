package decimal

import (
	"fmt"
	"math/bits"
)

// Add returns d + other exactly.
func (d Decimal) Add(other Decimal) (Decimal, error) {
	return d.add(other, "add")
}

func (d Decimal) add(other Decimal, op string) (Decimal, error) {
	aNeg, aScale, aMant := d.unpack()
	bNeg, bScale, bMant := other.unpack()

	if aMant.isZero() {
		return pack(bNeg, bScale, bMant), nil
	}
	if bMant.isZero() {
		return pack(aNeg, aScale, aMant), nil
	}

	scale := max(aScale, bScale)
	a, ok := scaleUp(aMant, int(scale-aScale))
	if !ok {
		return Decimal{}, fmt.Errorf("decimal: %s: %w", op, ErrOverflow)
	}
	b, ok := scaleUp(bMant, int(scale-bScale))
	if !ok {
		return Decimal{}, fmt.Errorf("decimal: %s: %w", op, ErrOverflow)
	}

	if aNeg == bNeg {
		sum, ok := a.add(b)
		if !ok {
			return Decimal{}, fmt.Errorf("decimal: %s: %w", op, ErrOverflow)
		}
		return exactPack(aNeg, int(scale), uint384FromUint192(sum), op)
	}

	switch a.cmp(b) {
	case 0:
		return Decimal{}, nil
	case 1:
		diff, _ := a.sub(b)
		return exactPack(aNeg, int(scale), uint384FromUint192(diff), op)
	default:
		diff, _ := b.sub(a)
		return exactPack(bNeg, int(scale), uint384FromUint192(diff), op)
	}
}

// Sub returns d - other exactly.
func (d Decimal) Sub(other Decimal) (Decimal, error) {
	return d.add(other.Neg(), "sub")
}

// Mul returns d * other exactly.
func (d Decimal) Mul(other Decimal) (Decimal, error) {
	aNeg, aScale, aMant := d.unpack()
	bNeg, bScale, bMant := other.unpack()
	if aMant.isZero() || bMant.isZero() {
		return Decimal{}, nil
	}

	return exactPack(aNeg != bNeg, int(aScale)+int(bScale), uint384FromUint192(fullMul96(aMant, bMant)), "mul")
}

// Div returns d / other exactly.
func (d Decimal) Div(other Decimal) (Decimal, error) {
	aNeg, aScale, aMant := d.unpack()
	bNeg, bScale, bMant := other.unpack()
	if bMant.isZero() {
		return Decimal{}, fmt.Errorf("decimal: div: %w", ErrDivideByZero)
	}
	if aMant.isZero() {
		return Decimal{}, nil
	}

	neg := aNeg != bNeg
	if bMant == oneMantissa && bScale == 0 {
		return pack(neg, aScale, aMant).Normalize(), nil
	}
	if exponent, ok := powerOfTenDivisorExponent(bMant, bScale); ok {
		targetScale := int(aScale) + exponent
		if targetScale < 0 {
			mantissa, ok := mulPow10Mantissa(aMant, uint8FromScale(-targetScale))
			if !ok {
				return Decimal{}, fmt.Errorf("decimal: div: %w", ErrOverflow)
			}
			return pack(neg, 0, mantissa), nil
		}
		res, err := exactPack(neg, targetScale, uint384FromMantissa(aMant), "div")
		if err != nil {
			return Decimal{}, err
		}
		return res.Normalize(), nil
	}
	if bMant.hi == 0 && !smallDivisorCanTerminate(aMant, bMant.lo) {
		return Decimal{}, fmt.Errorf("decimal: div: %w", ErrInexact)
	}

	denominator := uint384FromMantissa(bMant)
	var ok bool
	denominator, ok = denominator.scaleUp10(int(aScale))
	if !ok {
		return Decimal{}, fmt.Errorf("decimal: div: %w", ErrOverflow)
	}
	for scale := 0; scale <= int(MaxScale); scale++ {
		numerator := uint384FromMantissa(aMant)
		numerator, ok = numerator.scaleUp10(int(bScale) + scale)
		if !ok {
			break
		}
		quotient, remainder := divModWide(numerator, denominator)
		if !remainder.isZero() {
			if scale == 0 {
				if quotient.cmp(uint384FromMantissa(maxMantissa)) >= 0 {
					return Decimal{}, fmt.Errorf("decimal: div: %w", ErrOverflow)
				}
			}
			continue
		}
		res, err := exactPack(neg, scale, quotient, "div")
		if err != nil {
			return Decimal{}, err
		}
		return res.Normalize(), nil
	}
	return Decimal{}, fmt.Errorf("decimal: div: %w", ErrInexact)
}

func smallDivisorCanTerminate(dividend Mantissa, divisor uint64) bool {
	divisor >>= bits.TrailingZeros64(divisor)
	for divisor%5 == 0 {
		divisor /= 5
	}
	if divisor == 1 {
		return true
	}
	_, remainder := dividend.divMod64(divisor)
	return remainder == 0
}

// Remainder returns d % other. The result has the sign of d and the common
// operand scale.
func (d Decimal) Remainder(other Decimal) (Decimal, error) {
	aNeg, aScale, aMant := d.unpack()
	_, bScale, bMant := other.unpack()
	if bMant.isZero() {
		return Decimal{}, fmt.Errorf("decimal: remainder: %w", ErrDivideByZero)
	}
	if aMant.isZero() {
		return Decimal{}, nil
	}

	scale := max(aScale, bScale)
	numerator, ok := scaleUp(aMant, int(scale-aScale))
	if !ok {
		panic("decimal: internal remainder numerator overflow")
	}

	var divisor Mantissa
	if scale == bScale {
		divisor = bMant
	} else if scaledDivisor, ok := mulPow10Mantissa(bMant, scale-bScale); ok {
		divisor = scaledDivisor
	} else {
		return pack(aNeg, scale, aMant), nil
	}

	var remainder Mantissa
	if divisor.hi == 0 {
		var r uint64
		_, r = numerator.divModSmall(divisor.lo)
		remainder = MantissaFromUint64(r)
	} else {
		_, remainder = divMod192By96(numerator, divisor)
	}
	return pack(aNeg, scale, remainder), nil
}

func powerOfTenDivisorExponent(m Mantissa, scale uint8) (int, bool) {
	if m == tenMantissa {
		return 1 - int(scale), true
	}
	power := 0
	for {
		if m == oneMantissa {
			return power - int(scale), true
		}
		q, r := m.divMod64(10)
		if r != 0 {
			return 0, false
		}
		m = q
		power++
	}
}

func exactPack(negative bool, scale int, n uint384, op string) (Decimal, error) {
	if n.isZero() {
		return Decimal{}, nil
	}
	if scale <= int(MaxScale) {
		if mantissa, fits := n.mantissa(); fits {
			return pack(negative, uint8FromScale(scale), mantissa), nil
		}
	}
	maxAtScale, ok := uint384FromMantissa(maxMantissa).scaleUp10(scale)
	if !ok || n.cmp(maxAtScale) > 0 {
		return Decimal{}, fmt.Errorf("decimal: %s: %w", op, ErrOverflow)
	}
	for scale > int(MaxScale) {
		var rem uint64
		n, rem = n.divModSmall(10)
		if rem != 0 {
			return Decimal{}, fmt.Errorf("decimal: %s: %w", op, ErrInexact)
		}
		scale--
	}
	for {
		if mantissa, fits := n.mantissa(); fits {
			return pack(negative, uint8FromScale(scale), mantissa), nil
		}
		if scale == 0 {
			return Decimal{}, fmt.Errorf("decimal: %s: %w", op, ErrOverflow)
		}
		var rem uint64
		n, rem = n.divModSmall(10)
		if rem != 0 {
			return Decimal{}, fmt.Errorf("decimal: %s: %w", op, ErrInexact)
		}
		scale--
	}
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

// uint192 is the private scratch width for decimal arithmetic. Decimal stores
// a 96-bit mantissa; multiplying two mantissas or aligning scales can need
// 192 bits before the result is checked for exact representability.
type uint192 struct {
	hi  uint64
	mid uint64
	lo  uint64
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
