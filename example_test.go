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
	fixed, ok := decimal.FromUint64(10).Rescale(5)
	fmt.Println(fixed.String(), ok)

	// Output:
	// 10.00000 true
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
