package decimal

import (
	"errors"
	"fmt"
	"strconv"
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

	parsed, err := scanDecimalString(s)
	if err != nil {
		return Decimal{}, err
	}

	intPart := parsed.intPart
	fracPart := parsed.fracPart
	start := parsed.start
	end := parsed.end
	scale, ok := parseScale(len(fracPart), parsed.exp)
	if start == end {
		return Decimal{}, nil
	}
	if !ok {
		if parsed.exp < 0 {
			return Decimal{}, parseError(ErrInvalid)
		}
		return Decimal{}, parseError(ErrOverflow)
	}

	if scale >= 0 {
		for scale > int64(MaxScale) || (scale > 0 && !digitsFit(intPart, fracPart, start, end)) {
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

type parsedDecimalString struct {
	intPart  string
	fracPart string
	exp      int64
	start    int
	end      int
}

func scanDecimalString(s string) (parsedDecimalString, error) {
	exp := int64(0)
	expIdx := -1
	for i := range len(s) {
		if s[i] == 'e' || s[i] == 'E' {
			expIdx = i
			break
		}
	}
	if expIdx >= 0 {
		expPart := s[expIdx+1:]
		if expPart == "" {
			return parsedDecimalString{}, wrapParse("missing exponent")
		}
		n, err := strconv.ParseInt(expPart, 10, 64)
		if err != nil {
			return parsedDecimalString{}, wrapParse("invalid exponent")
		}
		exp = n
		s = s[:expIdx]
	}

	point := -1
	start := -1
	digits := 0
	for i := range len(s) {
		c := s[i]
		if c == '.' {
			if point >= 0 {
				return parsedDecimalString{}, wrapParse("multiple decimal points")
			}
			point = i
			continue
		}
		if c < '0' || c > '9' {
			return parsedDecimalString{}, wrapParse("invalid digit")
		}
		if c != '0' && start < 0 {
			start = digits
		}
		digits++
	}
	if digits == 0 {
		return parsedDecimalString{}, wrapParse("missing digits")
	}
	if start < 0 {
		start = digits
	}

	parsed := parsedDecimalString{
		exp:   exp,
		start: start,
		end:   digits,
	}
	if point >= 0 {
		parsed.intPart = s[:point]
		parsed.fracPart = s[point+1:]
	} else {
		parsed.intPart = s
	}
	return parsed, nil
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
	next, overflow := n.mul10Add(digit - '0')
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
