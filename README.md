# decimal

Pure Go fixed-size decimal package.

`Decimal` is a 128-bit value type backed by a 96-bit unsigned mantissa and a
32-bit flags word. Core parsing and arithmetic use fixed-size integer
arithmetic, not `math/big`.

Successful parsing and arithmetic do not allocate heap objects. Benchmarks for
those paths should report `0 B/op` and `0 allocs/op`. APIs that return Go
strings allocate for the returned string; append-style APIs allocate only if the
caller-provided buffer lacks capacity.

## Quickstart

```go
d, err := decimal.Parse("10.00000")
if err != nil {
	panic(err)
}

fmt.Println(d.String())
fmt.Println(d.Normalize().String())

fixed, err := decimal.FromUint64(10).Rescale(5)
if err != nil {
	panic(err)
}
fmt.Println(fixed.String())
```

Output:

```text
10.00000
10
10.00000
```

## Model and Scale

The numeric value is:

```text
value = (-1)^sign * mantissa * 10^-scale
```

Scale is the number of fractional decimal digits and is always in `[0, 28]`.
The layout was chosen with .NET decimal as a reference, but this package defines
its own Go API and behavior.

`Bits` and `FromBits` expose the representation in `lo, mid, hi, flags` word
order:

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

Parsing accepts an optional sign, decimal digits with an optional decimal point,
and an optional base-10 exponent. Either side of the decimal point may be empty
when the other side has digits, so `".01"`, `"-.01"`, and `"1."` are valid.

Formatting preserves stored trailing zeroes for non-zero values. `StringFixed`
and `AppendFixed` format with an exact requested scale without rounding.
`Decimal` also implements `fmt.Formatter`, so `fmt.Sprintf("%.2f", d)` emits
exactly two fractional digits when the value can be represented at that scale.

## Exact Arithmetic

`Add`, `Sub`, `Mul`, `Div`, `Remainder`, and `Rescale` return `(Decimal,
error)`. Arithmetic is exact by default. If the exact result cannot be
represented, the operation returns a wrapped sentinel error:

- `ErrInexact`: non-zero precision would be lost.
- `ErrOverflow`: the numeric result is outside the representable range.
- `ErrDivideByZero`: division or remainder by zero.
- `ErrInvalid`: invalid scale, mode, quantum, or clamp bounds.

Use `errors.Is` to inspect errors.

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
- `RoundAwayFromZero`: any discarded non-zero digits increase magnitude.
- `RoundHalfTowardZero`: halfway cases round toward zero.

`Quantize(scale, mode)` rounds to exactly `scale` fractional digits for non-zero
results. `QuantizeTo(quantum, mode)` rounds to a multiple of a positive non-zero
decimal quantum such as `0.05`; the quantum's stored scale controls the
returned non-zero scale.

`MulToScale`, `DivToScale`, `MulQuantized`, `DivQuantized`,
`MulAddToScale`, and `MulAddQuantized` perform one-round policy arithmetic.
Use these instead of rounding intermediate results.

## Verification

```sh
go test ./...
go test -race ./...
go test -bench=. -benchmem ./...
```
