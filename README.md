# decimal

Pure Go fixed-size decimal package.

## Quickstart

```go
d, err := decimal.Parse("10.00000")
if err != nil {
	panic(err)
}

fmt.Println(d.String())
fmt.Println(d.Normalize().String())

fixed, ok := decimal.FromUint64(10).Rescale(5)
fmt.Println(fixed.String(), ok)
```

Output:

```text
10.00000
10
10.00000 true
```

## Model

`Decimal` is a signed 128-bit decimal value:

```text
value = (-1)^sign * mantissa * 10^-scale
```

Stored representation:

```text
128-bit Decimal
+------------------------------------------------+-------------------------------+
|                96-bit mantissa                 |          32-bit flags          |
+------------------------------------------------+-------------------------------+
| hi: bits 95..64 | mid: bits 63..32 | lo: 31..0 | sign | reserved | scale | res |
+------------------------------------------------+-------------------------------+

Bits()/FromBits() word order:

    lo, mid, hi, flags

flags:

    bits  0..15  reserved, must be zero
    bits 16..23  scale, 0..28
    bits 24..30  reserved, must be zero
    bit      31  sign
```

The 96-bit mantissa plus flags layout was chosen with .NET decimal as a
reference. This package defines its own Go API and behavior.

Example:

```go
d, err := decimal.Parse("10.00000")
if err != nil {
	panic(err)
}

lo, mid, hi, flags := d.Bits()
fmt.Printf("lo=%d mid=%d hi=%d flags=0x%08x\n", lo, mid, hi, flags)
```

Output:

```text
lo=1000000 mid=0 hi=0 flags=0x00050000
```

`10.00000` is stored as mantissa `1000000` with scale `5`. `10` is stored as
mantissa `10` with scale `0`. The values compare numerically equal, but their
representations differ.

Invariants:

- `scale` is in `[0, 28]`.
- `mantissa` is at most `2^96 - 1`.
- zero is canonicalized to positive scale `0`.
- `Decimal` is comparable, but `==` compares the stored representation. Use
  `Equal` or `Cmp` for numeric equality.
- runtime arithmetic uses fixed-size integer code, not `math/big`.
- tests may use `math/big` as an oracle.

## Formatting and Normalization

`String` and `AppendText` preserve the stored scale for non-zero values. They do
not normalize trailing fractional zeroes.

```go
d, _ := decimal.Parse("10.00000")
fmt.Println(d.String())
fmt.Println(d.Normalize().String())
```

Output:

```text
10.00000
10
```

Use `Normalize` when insignificant trailing fractional zeroes should be removed
from the representation. Use `Rescale` when a fixed number of fractional digits
is required:

```go
fixed, ok := decimal.FromUint64(10).Rescale(5)
fmt.Println(fixed.String(), ok)
```

Output:

```text
10.00000 true
```

Zero is always canonicalized and formats as `"0"`, even if parsed from `"-0"` or
`"0.00000"`.

General formatting preserves stored trailing zeroes. This package does not
provide custom format strings; compose `Normalize`, `Rescale`, and `String`
instead.

## API

GoDoc is the source of truth for the full exported API. The contracts below are
the parts that matter for correctness.

`Mantissa` word constructors and `Words` expose numeric limbs for constructing
or inspecting values that do not fit in `uint64`. `hi` is bits 95:64 and `lo`
is bits 63:0; this is not byte order.

Arithmetic methods return `(Decimal, bool)`, where `bool` is `true` when the
result is representable. `Add`, `Sub`, and `Mul` use the operation's natural
scale, then round half away from zero while reducing scale only when the
mantissa would not fit. `Div` rounds half away from zero to at most `MaxScale`
fractional digits. `Remainder` preserves the common operand scale for non-zero
results and uses the dividend sign. `false` means no representable result
exists, division by zero, or remainder by zero.

`Round(scale, mode)` rounds to at most `scale` fractional digits. It returns the
value unchanged when the current scale is already less than or equal to
`scale`; it does not add trailing zeroes. The rounding mode controls only what
happens when fractional digits are discarded:

- `RoundHalfAwayFromZero`: halfway cases round away from zero.
- `RoundHalfToEven`: halfway cases round to the nearest even final digit.
- `RoundTowardZero`: discarded digits never increase the magnitude.
- `RoundTowardNegativeInfinity`: discarded digits round toward negative
  infinity.
- `RoundTowardPositiveInfinity`: discarded digits round toward positive
  infinity.

`Round` returns `false` when `scale` or `mode` is invalid, or when the rounded
mantissa cannot be represented. `RoundTo(scale)` is shorthand for
`Round(scale, RoundHalfAwayFromZero)` when callers want the default rounding
mode and do not need the success flag.

Integer conversions truncate toward zero and return `false` when the truncated
value does not fit the target integer type.

Operations that can fail return a boolean instead of panicking. The package has
no checked/unchecked mode split; call sites decide whether `false` should be
propagated, converted to an error, or treated as a panic.

`Parse` errors match `ErrParse`; range failures also match `ErrInvalid` or
`ErrOverflow`.

JSON marshals as a quoted decimal string. JSON unmarshal accepts quoted decimal
strings and JSON numbers. SQL `Value` returns a decimal string; `Scan` accepts
`nil`, `string`, and `[]byte`.

## Verification

```sh
go test ./...
go test -race ./...
go test -run=^$ -fuzz=FuzzDecimalArithmeticWithBigRatOracle -fuzztime=10s ./...
go test -bench=. -benchmem ./...
```
