package decimal

import (
	"database/sql/driver"
	"fmt"
)

// Scan implements sql.Scanner. It accepts nil, string, and []byte values; nil
// scans as the canonical zero.
func (d *Decimal) Scan(value any) error {
	if value == nil {
		*d = Decimal{}
		return nil
	}

	switch v := value.(type) {
	case string:
		parsed, err := Parse(v)
		if err != nil {
			return err
		}
		*d = parsed
		return nil
	case []byte:
		parsed, err := Parse(string(v))
		if err != nil {
			return err
		}
		*d = parsed
		return nil
	default:
		return fmt.Errorf("decimal: cannot scan from %T: %w", value, ErrParse)
	}
}

// Value implements driver.Valuer. It returns d formatted as a decimal string.
func (d Decimal) Value() (driver.Value, error) {
	return d.String(), nil
}
