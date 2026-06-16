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

## Model and Scale

`Decimal` is a signed 128-bit decimal value:

```text
value = (-1)^sign * mantissa * 10^-scale
```

It stores a 96-bit mantissa and a 32-bit flags word containing sign and scale.
The layout was chosen with .NET decimal as a reference, but this package defines
its own Go API and behavior. `Bits` and `FromBits` use `lo, mid, hi, flags`
word order.

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
representations differ. `Decimal` is intentionally non-comparable; use `Equal`
or `Cmp` for numeric equality, and `Bits` when representation equality is
required.

Zero is always canonicalized and formats as `"0"`, even if parsed from `"-0"` or
`"0.00000"`.

General formatting preserves stored trailing zeroes. This package does not
provide custom format strings; compose `Normalize`, `Rescale`, and `String`
instead.

## Rounding

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

## Verification

```sh
go test ./...
go test -race ./...
go test -run=^$ -fuzz=FuzzDecimalArithmeticWithBigRatOracle -fuzztime=10s ./...
go test -bench=. -benchmem ./...
```
