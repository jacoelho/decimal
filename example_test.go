package decimal_test

import (
	"fmt"

	"github.com/jacoelho/decimal"
)

func ExampleParse_formatting() {
	d, err := decimal.Parse("10.00000")
	if err != nil {
		panic(err)
	}

	fmt.Println(d.String())
	fmt.Println(d.Normalize().String())

	// Output:
	// 10.00000
	// 10
}

func ExampleDecimal_Rescale() {
	fixed, err := decimal.FromUint64(10).Rescale(5)
	if err != nil {
		panic(err)
	}
	fmt.Println(fixed.String())

	// Output:
	// 10.00000
}

func ExampleDecimal_ScaleByPowerOfTen() {
	d, err := decimal.Parse("1.2300")
	if err != nil {
		panic(err)
	}
	for _, n := range []int{2, -2} {
		shifted, err := d.ScaleByPowerOfTen(n)
		if err != nil {
			panic(err)
		}
		fmt.Println(shifted.String())
	}

	// Output:
	// 123.00
	// 0.012300
}

func ExampleDecimal_Quantize() {
	d, err := decimal.Parse("10.005")
	if err != nil {
		panic(err)
	}
	fixed, err := d.Quantize(2, decimal.RoundHalfToEven)
	if err != nil {
		panic(err)
	}
	fmt.Println(fixed.String())

	// Output:
	// 10.00
}

func ExampleDecimal_Bits() {
	d, err := decimal.Parse("10.00000")
	if err != nil {
		panic(err)
	}

	lo, mid, hi, flags := d.Bits()
	fmt.Printf("lo=%d mid=%d hi=%d flags=0x%08x\n", lo, mid, hi, flags)

	// Output:
	// lo=1000000 mid=0 hi=0 flags=0x00050000
}
