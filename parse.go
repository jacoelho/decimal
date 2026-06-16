package decimal

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrParse is returned when Parse cannot read a decimal.
var ErrParse = errors.New("decimal: parse error")

// Parse parses a decimal string.
//
// Accepted forms are an optional sign, decimal digits with an optional decimal
// point, and an optional base-10 exponent. The stored scale cannot exceed
// MaxScale and the mantissa must fit in 96 bits.
func Parse(s string) (Decimal, error) {
	if s == "" {
		return Decimal{}, wrapParse("empty string")
	}

	negative := false
	if s[0] == '-' || s[0] == '+' {
		negative = s[0] == '-'
		s = s[1:]
		if s == "" {
			return Decimal{}, wrapParse("sign without value")
		}
	}

	exp := int64(0)
	if idx := strings.IndexAny(s, "eE"); idx >= 0 {
		expPart := s[idx+1:]
		if expPart == "" {
			return Decimal{}, wrapParse("missing exponent")
		}
		n, err := strconv.ParseInt(expPart, 10, 64)
		if err != nil {
			return Decimal{}, wrapParse("invalid exponent")
		}
		exp = n
		s = s[:idx]
	}

	intPart, fracPart, hasPoint := strings.Cut(s, ".")
	if hasPoint && strings.Contains(fracPart, ".") {
		return Decimal{}, wrapParse("multiple decimal points")
	}
	if intPart == "" && fracPart == "" {
		return Decimal{}, wrapParse("missing digits")
	}

	scale, ok := parseScale(len(fracPart), exp)
	start, end, err := significantDigits(intPart, fracPart)
	if err != nil {
		return Decimal{}, err
	}
	if start == end {
		return Decimal{}, nil
	}
	if !ok {
		if exp < 0 {
			return Decimal{}, parseError(ErrInvalid)
		}
		return Decimal{}, parseError(ErrOverflow)
	}

	if scale < 0 {
		if !digitsFit(intPart, fracPart, start, end) {
			return Decimal{}, parseError(ErrOverflow)
		}
	} else {
		for scale > int64(MaxScale) || !digitsFit(intPart, fracPart, start, end) {
			if scale == 0 {
				return Decimal{}, parseError(ErrOverflow)
			}
			if lastDigit(intPart, fracPart, end) != '0' {
				if scale > int64(MaxScale) {
					return Decimal{}, parseError(ErrInvalid)
				}
				return Decimal{}, parseError(ErrOverflow)
			}
			end--
			scale--
		}
	}

	mantissa, err := parseMantissa(intPart, fracPart, start, end)
	if err != nil {
		return Decimal{}, parseError(err)
	}
	if scale < 0 {
		mulBy := -scale
		if mulBy > int64(MaxScale) {
			return Decimal{}, parseError(ErrOverflow)
		}
		var scaled bool
		mantissa, scaled = mulPow10Mantissa(mantissa, uint8(mulBy))
		if !scaled {
			return Decimal{}, parseError(ErrOverflow)
		}
		scale = 0
	}

	d, ok := fromParts(mantissa, uint8(scale), negative)
	if !ok {
		return Decimal{}, parseError(ErrOverflow)
	}
	return d, nil
}

func parseScale(fracDigits int, exp int64) (int64, bool) {
	const maxInt64 = 1<<63 - 1
	if exp < 0 {
		if int64(fracDigits) > maxInt64+exp {
			return 0, false
		}
		return int64(fracDigits) - exp, true
	}
	return int64(fracDigits) - exp, true
}

func significantDigits(intPart, fracPart string) (int, int, error) {
	total := len(intPart) + len(fracPart)
	start := total
	digits := 0

	for _, part := range [...]string{intPart, fracPart} {
		for i := range len(part) {
			c := part[i]
			if c < '0' || c > '9' {
				return 0, 0, wrapParse("invalid digit")
			}
			if c != '0' && start == total {
				start = digits
			}
			digits++
		}
	}
	if digits == 0 {
		return 0, 0, wrapParse("missing digits")
	}
	if start == total {
		return total, total, nil
	}
	return start, total, nil
}

func digitsFit(intPart, fracPart string, start, end int) bool {
	n := end - start
	if n != len(maxMantissaDecimal) {
		return n < len(maxMantissaDecimal)
	}

	pos := 0
	if start < len(intPart) {
		intEnd := min(end, len(intPart))
		for i := start; i < intEnd; i++ {
			c := intPart[i]
			if c != maxMantissaDecimal[pos] {
				return c < maxMantissaDecimal[pos]
			}
			pos++
		}
	}
	if end > len(intPart) {
		fracStart := max(start-len(intPart), 0)
		fracEnd := end - len(intPart)
		for i := fracStart; i < fracEnd; i++ {
			c := fracPart[i]
			if c != maxMantissaDecimal[pos] {
				return c < maxMantissaDecimal[pos]
			}
			pos++
		}
	}
	return true
}

func parseMantissa(intPart, fracPart string, start, end int) (Mantissa, error) {
	var n Mantissa
	if start < len(intPart) {
		intEnd := min(end, len(intPart))
		for i := start; i < intEnd; i++ {
			var err error
			n, err = addMantissaDigit(n, intPart[i])
			if err != nil {
				return Mantissa{}, err
			}
		}
	}
	if end > len(intPart) {
		fracStart := max(start-len(intPart), 0)
		fracEnd := end - len(intPart)
		for i := fracStart; i < fracEnd; i++ {
			var err error
			n, err = addMantissaDigit(n, fracPart[i])
			if err != nil {
				return Mantissa{}, err
			}
		}
	}
	return n, nil
}

func lastDigit(intPart, fracPart string, end int) byte {
	if end <= len(intPart) {
		return intPart[end-1]
	}
	return fracPart[end-len(intPart)-1]
}

func addMantissaDigit(n Mantissa, digit byte) (Mantissa, error) {
	next, overflow := n.mulChecked(tenMantissa)
	if overflow {
		return Mantissa{}, ErrOverflow
	}
	n = next
	next, overflow = n.addChecked(MantissaFromUint64(uint64(digit - '0')))
	if overflow {
		return Mantissa{}, ErrOverflow
	}
	return next, nil
}

func wrapParse(msg string) error {
	return fmt.Errorf("%w: %s", ErrParse, msg)
}

func parseError(err error) error {
	return fmt.Errorf("%w: %w", ErrParse, err)
}
