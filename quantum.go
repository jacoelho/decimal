package decimal

import "fmt"

// Quantize returns d rounded to exactly scale fractional digits using mode.
// Non-zero results have exactly scale; zero remains canonical.
func (d Decimal) Quantize(scale uint8, mode RoundingMode) (Decimal, error) {
	quantum, err := quantumFromScale(scale)
	if err != nil {
		return Decimal{}, err
	}
	return d.QuantizeTo(quantum, mode)
}

// QuantizeTo returns d rounded to a multiple of quantum using mode. Quantum
// must be positive and non-zero. The stored scale of quantum controls the
// returned scale for non-zero results.
func (d Decimal) QuantizeTo(quantum Decimal, mode RoundingMode) (Decimal, error) {
	negative, scale, mantissa := d.unpack()
	denominator, ok := pow10Uint384(int(scale))
	if !ok {
		return Decimal{}, fmt.Errorf("decimal: quantize: %w", ErrOverflow)
	}
	return quantizeRational("quantize", negative, uint384FromMantissa(mantissa), denominator, quantum, mode)
}

// MulQuantized returns d * other rounded once to a multiple of quantum.
func (d Decimal) MulQuantized(other, quantum Decimal, mode RoundingMode) (Decimal, error) {
	aNeg, aScale, aMant := d.unpack()
	bNeg, bScale, bMant := other.unpack()
	denominator, ok := pow10Uint384(int(aScale) + int(bScale))
	if !ok {
		return Decimal{}, fmt.Errorf("decimal: mul quantized: %w", ErrOverflow)
	}
	if aMant.isZero() || bMant.isZero() {
		return quantizeRational("mul quantized", false, uint384{}, denominator, quantum, mode)
	}
	return quantizeRational(
		"mul quantized",
		aNeg != bNeg,
		uint384FromUint192(fullMul96(aMant, bMant)),
		denominator,
		quantum,
		mode,
	)
}

// MulToScale returns d * other rounded once to exactly scale fractional digits.
func (d Decimal) MulToScale(other Decimal, scale uint8, mode RoundingMode) (Decimal, error) {
	quantum, err := quantumFromScale(scale)
	if err != nil {
		return Decimal{}, err
	}
	return d.MulQuantized(other, quantum, mode)
}

// DivQuantized returns d / other rounded once to a multiple of quantum.
func (d Decimal) DivQuantized(other, quantum Decimal, mode RoundingMode) (Decimal, error) {
	aNeg, aScale, aMant := d.unpack()
	bNeg, bScale, bMant := other.unpack()
	if bMant.isZero() {
		return Decimal{}, fmt.Errorf("decimal: div quantized: %w", ErrDivideByZero)
	}

	numerator := uint384FromMantissa(aMant)
	var ok bool
	numerator, ok = numerator.scaleUp10(int(bScale))
	if !ok {
		return Decimal{}, fmt.Errorf("decimal: div quantized: %w", ErrOverflow)
	}
	denominator := uint384FromMantissa(bMant)
	denominator, ok = denominator.scaleUp10(int(aScale))
	if !ok {
		return Decimal{}, fmt.Errorf("decimal: div quantized: %w", ErrOverflow)
	}
	return quantizeRational("div quantized", aNeg != bNeg, numerator, denominator, quantum, mode)
}

// DivToScale returns d / other rounded once to exactly scale fractional digits.
func (d Decimal) DivToScale(other Decimal, scale uint8, mode RoundingMode) (Decimal, error) {
	quantum, err := quantumFromScale(scale)
	if err != nil {
		return Decimal{}, err
	}
	return d.DivQuantized(other, quantum, mode)
}

// MulAddQuantized returns d*multiplier + addend rounded once to a multiple of
// quantum.
func (d Decimal) MulAddQuantized(multiplier, addend, quantum Decimal, mode RoundingMode) (Decimal, error) {
	aNeg, aScale, aMant := d.unpack()
	bNeg, bScale, bMant := multiplier.unpack()
	cNeg, cScale, cMant := addend.unpack()

	productNeg := aNeg != bNeg
	productScale := int(aScale) + int(bScale)
	product := uint384FromUint192(fullMul96(aMant, bMant))
	add := uint384FromMantissa(cMant)

	scale := max(productScale, int(cScale))
	var ok bool
	product, ok = product.scaleUp10(scale - productScale)
	if !ok {
		return Decimal{}, fmt.Errorf("decimal: muladd quantized: %w", ErrOverflow)
	}
	add, ok = add.scaleUp10(scale - int(cScale))
	if !ok {
		return Decimal{}, fmt.Errorf("decimal: muladd quantized: %w", ErrOverflow)
	}

	negative := productNeg
	var magnitude uint384
	if productNeg == cNeg {
		var overflow bool
		magnitude, overflow = product.add(add)
		if overflow {
			return Decimal{}, fmt.Errorf("decimal: muladd quantized: %w", ErrOverflow)
		}
	} else {
		switch product.cmp(add) {
		case 0:
			magnitude = uint384{}
			negative = false
		case 1:
			magnitude, _ = product.sub(add)
		default:
			magnitude, _ = add.sub(product)
			negative = cNeg
		}
	}

	denominator, ok := pow10Uint384(scale)
	if !ok {
		return Decimal{}, fmt.Errorf("decimal: muladd quantized: %w", ErrOverflow)
	}
	return quantizeRational("muladd quantized", negative, magnitude, denominator, quantum, mode)
}

// MulAddToScale returns d*multiplier + addend rounded once to exactly scale
// fractional digits.
func (d Decimal) MulAddToScale(multiplier, addend Decimal, scale uint8, mode RoundingMode) (Decimal, error) {
	quantum, err := quantumFromScale(scale)
	if err != nil {
		return Decimal{}, err
	}
	return d.MulAddQuantized(multiplier, addend, quantum, mode)
}

func quantumFromScale(scale uint8) (Decimal, error) {
	if scale > MaxScale {
		return Decimal{}, fmt.Errorf("decimal: quantum scale %d: %w", scale, ErrInvalid)
	}
	return pack(false, scale, oneMantissa), nil
}

func validQuantum(quantum Decimal) (Mantissa, uint8, error) {
	mantissa, scale, negative := quantum.Parts()
	if negative || mantissa.isZero() {
		return Mantissa{}, 0, fmt.Errorf("decimal: quantum: %w", ErrInvalid)
	}
	return mantissa, scale, nil
}

func quantizeRational(op string, negative bool, numerator, denominator uint384, quantum Decimal, mode RoundingMode) (Decimal, error) {
	quantumMantissa, quantumScale, err := validQuantum(quantum)
	if err != nil {
		return Decimal{}, err
	}
	if !validRoundingMode(mode) || denominator.isZero() {
		return Decimal{}, fmt.Errorf("decimal: %s: %w", op, ErrInvalid)
	}
	if numerator.isZero() {
		return Decimal{}, nil
	}

	scaledNumerator, ok := numerator.scaleUp10(int(quantumScale))
	if !ok {
		return Decimal{}, fmt.Errorf("decimal: %s: %w", op, ErrOverflow)
	}
	var overflow bool
	if quantumMantissa == oneMantissa {
		quotient, remainder := divModWide(scaledNumerator, denominator)
		if mode != RoundTowardZero && shouldRoundUpWide(mode, negative, quotient, remainder, denominator) {
			quotient, overflow = quotient.addOne()
			if overflow {
				return Decimal{}, fmt.Errorf("decimal: %s: %w", op, ErrOverflow)
			}
		}
		return packFixedScale(negative, quantumScale, quotient, op)
	}

	scaledDenominator, overflow := denominator.mulMantissa(quantumMantissa)
	if overflow {
		return Decimal{}, fmt.Errorf("decimal: %s: %w", op, ErrOverflow)
	}

	quotient, remainder := divModWide(scaledNumerator, scaledDenominator)
	if mode != RoundTowardZero && shouldRoundUpWide(mode, negative, quotient, remainder, scaledDenominator) {
		quotient, overflow = quotient.addOne()
		if overflow {
			return Decimal{}, fmt.Errorf("decimal: %s: %w", op, ErrOverflow)
		}
	}

	result, overflow := quotient.mulMantissa(quantumMantissa)
	if overflow {
		return Decimal{}, fmt.Errorf("decimal: %s: %w", op, ErrOverflow)
	}
	return packFixedScale(negative, quantumScale, result, op)
}

func packFixedScale(negative bool, scale uint8, n uint384, op string) (Decimal, error) {
	if n.isZero() {
		return Decimal{}, nil
	}
	mantissa, ok := n.mantissa()
	if !ok {
		return Decimal{}, fmt.Errorf("decimal: %s: %w", op, ErrOverflow)
	}
	return pack(negative, scale, mantissa), nil
}

func shouldRoundUpWide(mode RoundingMode, negative bool, quotient, remainder, divisor uint384) bool {
	if remainder.isZero() {
		return false
	}
	switch mode {
	case RoundHalfAwayFromZero:
		cmp := compareTwice(remainder, divisor)
		return cmp >= 0
	case RoundHalfToEven:
		cmp := compareTwice(remainder, divisor)
		return cmp > 0 || cmp == 0 && quotient.w[0]&1 == 1
	case RoundTowardZero:
		return false
	case RoundTowardNegativeInfinity:
		return negative
	case RoundTowardPositiveInfinity:
		return !negative
	case RoundAwayFromZero:
		return true
	case RoundHalfTowardZero:
		return compareTwice(remainder, divisor) > 0
	default:
		panic("decimal: invalid rounding mode")
	}
}

func compareTwice(a, b uint384) int {
	if b.w[1]|b.w[2]|b.w[3]|b.w[4]|b.w[5] == 0 {
		if a.w[1]|a.w[2]|a.w[3]|a.w[4]|a.w[5] != 0 || a.w[0]>>63 != 0 {
			return 1
		}
		double := a.w[0] << 1
		if double < b.w[0] {
			return -1
		}
		if double > b.w[0] {
			return 1
		}
		return 0
	}
	double, overflow := a.add(a)
	if overflow {
		return 1
	}
	return double.cmp(b)
}
