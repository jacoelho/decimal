package decimal

import (
	"encoding"
	"strconv"
)

var (
	_ encoding.TextMarshaler   = Decimal{}
	_ encoding.TextAppender    = Decimal{}
	_ encoding.TextUnmarshaler = (*Decimal)(nil)
)

// MarshalJSON implements json.Marshaler. It encodes d as a quoted decimal
// string.
func (d Decimal) MarshalJSON() ([]byte, error) {
	var buf [82]byte
	out := append(buf[:0], '"')
	out = d.appendText(out)
	out = append(out, '"')
	return append([]byte(nil), out...), nil
}

// UnmarshalJSON implements json.Unmarshaler. It accepts a quoted decimal string
// or a JSON number.
func (d *Decimal) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return ErrParse
	}
	var s string
	if data[0] == '"' {
		unquoted, err := strconv.Unquote(string(data))
		if err != nil {
			return err
		}
		s = unquoted
	} else {
		s = string(data)
	}
	parsed, err := Parse(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// MarshalText implements encoding.TextMarshaler. It returns the same decimal
// form as String.
func (d Decimal) MarshalText() ([]byte, error) {
	var buf [80]byte
	out := d.appendText(buf[:0])
	return append([]byte(nil), out...), nil
}

// UnmarshalText implements encoding.TextUnmarshaler. It accepts the same
// syntax as Parse.
func (d *Decimal) UnmarshalText(data []byte) error {
	parsed, err := Parse(string(data))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
