package decimal

import "math/bits"

type uint384 struct {
	w [6]uint64
}

func uint384FromMantissa(m Mantissa) uint384 {
	return uint384{w: [6]uint64{m.lo, uint64(m.hi)}}
}

func uint384FromUint192(n uint192) uint384 {
	return uint384{w: [6]uint64{n.lo, n.mid, n.hi}}
}

func (n uint384) isZero() bool {
	return n.w[0]|n.w[1]|n.w[2]|n.w[3]|n.w[4]|n.w[5] == 0
}

func (n uint384) cmp(other uint384) int {
	for i := len(n.w) - 1; i >= 0; i-- {
		if n.w[i] != other.w[i] {
			if n.w[i] < other.w[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func (n uint384) bit(i int) uint64 {
	return n.w[i/64] >> uint(i%64) & 1
}

func (n uint384) lsh1() (uint384, bool) {
	var out uint384
	carry := uint64(0)
	for i := range n.w {
		next := n.w[i] >> 63
		out.w[i] = n.w[i]<<1 | carry
		carry = next
	}
	return out, carry != 0
}

func (n uint384) add(other uint384) (uint384, bool) {
	var out uint384
	carry := uint64(0)
	for i := range n.w {
		out.w[i], carry = bits.Add64(n.w[i], other.w[i], carry)
	}
	return out, carry != 0
}

func (n uint384) sub(other uint384) (uint384, bool) {
	var out uint384
	borrow := uint64(0)
	for i := range n.w {
		out.w[i], borrow = bits.Sub64(n.w[i], other.w[i], borrow)
	}
	return out, borrow != 0
}

func (n uint384) addOne() (uint384, bool) {
	var out uint384
	out.w = n.w
	for i := range out.w {
		out.w[i]++
		if out.w[i] != 0 {
			return out, false
		}
	}
	return uint384{}, true
}

func (n uint384) mulSmall(m uint64) (uint384, bool) {
	if m == 0 || n.isZero() {
		return uint384{}, false
	}
	var out uint384
	carry := uint64(0)
	for i := range n.w {
		hi, lo := bits.Mul64(n.w[i], m)
		lo, c := bits.Add64(lo, carry, 0)
		out.w[i] = lo
		carry = hi + c
		if carry < hi {
			return uint384{}, true
		}
	}
	return out, carry != 0
}

func (n uint384) mulMantissa(m Mantissa) (uint384, bool) {
	if m.isZero() || n.isZero() {
		return uint384{}, false
	}
	out, overflow := n.mulSmall(m.lo)
	if m.hi == 0 {
		return out, overflow
	}

	hiProduct, hiOverflow := n.mulSmall(uint64(m.hi))
	shifted, shiftOverflow := hiProduct.shiftWords(1)
	out, addOverflow := out.add(shifted)
	return out, overflow || hiOverflow || shiftOverflow || addOverflow
}

func (n uint384) shiftWords(words int) (uint384, bool) {
	if words == 0 || n.isZero() {
		return n, false
	}
	if words >= len(n.w) {
		return uint384{}, !n.isZero()
	}
	var out uint384
	copy(out.w[words:], n.w[:len(n.w)-words])
	for _, w := range n.w[len(n.w)-words:] {
		if w != 0 {
			return uint384{}, true
		}
	}
	return out, false
}

func (n uint384) scaleUp10(scale int) (uint384, bool) {
	if scale < 0 {
		panic("decimal: internal negative scale")
	}
	if scale == 0 {
		return n, true
	}
	if scale <= 19 {
		out, overflow := n.mulSmall(pow10[scale].lo)
		return out, !overflow
	}
	out := n
	for scale > 0 {
		step := min(scale, len(pow10)-1)
		var overflow bool
		if step <= 19 {
			out, overflow = out.mulSmall(pow10[step].lo)
		} else {
			out, overflow = out.mulMantissa(pow10[step])
		}
		if overflow {
			return uint384{}, false
		}
		scale -= step
	}
	return out, true
}

func pow10Uint384(scale int) (uint384, bool) {
	if scale >= 0 && scale < len(pow10) {
		return uint384FromMantissa(pow10[scale]), true
	}
	return uint384{w: [6]uint64{1}}.scaleUp10(scale)
}

func (n uint384) divModSmall(divisor uint64) (uint384, uint64) {
	if divisor == 0 {
		panic("decimal: internal divide by zero")
	}
	var out uint384
	rem := uint64(0)
	for i := len(n.w) - 1; i >= 0; i-- {
		out.w[i], rem = bits.Div64(rem, n.w[i], divisor)
	}
	return out, rem
}

func (n uint384) divMod(divisor uint384) (uint384, uint384) {
	if divisor.isZero() {
		panic("decimal: internal divide by zero")
	}
	if n.cmp(divisor) < 0 {
		return uint384{}, n
	}

	var quotient, remainder uint384
	for i := 383; i >= 0; i-- {
		var overflow bool
		remainder, overflow = remainder.lsh1()
		if overflow {
			panic("decimal: internal uint384 division overflow")
		}
		remainder.w[0] |= n.bit(i)
		if remainder.cmp(divisor) >= 0 {
			var borrow bool
			remainder, borrow = remainder.sub(divisor)
			if borrow {
				panic("decimal: internal uint384 division borrow")
			}
			quotient.w[i/64] |= uint64(1) << uint(i%64)
		}
	}
	return quotient, remainder
}

func (n uint384) mantissa() (Mantissa, bool) {
	if n.w[2] != 0 || n.w[3] != 0 || n.w[4] != 0 || n.w[5] != 0 || n.w[1] > maxMantissaHiWord {
		return Mantissa{}, false
	}
	return NewMantissa(uint32FromChecked(n.w[1]), n.w[0]), true
}

func (n uint384) uint64() (uint64, bool) {
	if n.w[1] != 0 || n.w[2] != 0 || n.w[3] != 0 || n.w[4] != 0 || n.w[5] != 0 {
		return 0, false
	}
	return n.w[0], true
}

func (n uint384) uint192() (uint192, bool) {
	if n.w[3] != 0 || n.w[4] != 0 || n.w[5] != 0 {
		return uint192{}, false
	}
	return uint192{hi: n.w[2], mid: n.w[1], lo: n.w[0]}, true
}

func divModWide(n, divisor uint384) (uint384, uint384) {
	if d, ok := divisor.uint64(); ok {
		if n192, ok := n.uint192(); ok {
			q, r := n192.divModSmall(d)
			return uint384FromUint192(q), uint384{w: [6]uint64{r}}
		}
		q, r := n.divModSmall(d)
		return q, uint384{w: [6]uint64{r}}
	}

	n192, nOK := n.uint192()
	dMant, dOK := divisor.mantissa()
	if nOK && dOK {
		q, r := divMod192By96(n192, dMant)
		return uint384FromUint192(q), uint384FromMantissa(r)
	}

	return n.divMod(divisor)
}
