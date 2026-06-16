// Package decimal provides a fixed-size signed decimal type.
//
// Decimal stores a 96-bit unsigned mantissa and a .NET-compatible 32-bit flags
// word carrying sign and scale. The value is mantissa * 10^-scale, negated when
// the sign bit is set. Decimal is comparable, but == compares representation;
// use Equal or Cmp for numeric equality.
//
// Runtime operations use fixed-size pure Go integer arithmetic. Tests use
// math/big only as an oracle.
package decimal
