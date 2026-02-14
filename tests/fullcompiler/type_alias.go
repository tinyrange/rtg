package main

import (
	"fmt"
	"os"
)

type MyInt int

func (m MyInt) Double() MyInt {
	return m * 2
}

type Celsius int
type Fahrenheit int

func main() {
	passed := true

	var x MyInt = 5
	if x.Double() != 10 {
		fmt.Printf("FAIL: method on named type\n")
		passed = false
	}

	// Conversion between named type and underlying type
	var n int = int(x)
	if n != 5 {
		fmt.Printf("FAIL: named to underlying\n")
		passed = false
	}

	var y MyInt = MyInt(42)
	if y != 42 {
		fmt.Printf("FAIL: underlying to named\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
