package main

import (
	"fmt"
	"os"
)

const (
	Zero = iota
	One
	Two
	Three
)

const (
	Bit0 = 1 << iota
	Bit1
	Bit2
	Bit3
)

func main() {
	passed := true

	if Zero != 0 || One != 1 || Two != 2 || Three != 3 {
		fmt.Printf("FAIL: basic iota Zero=%d One=%d Two=%d Three=%d\n", Zero, One, Two, Three)
		passed = false
	}

	if Bit0 != 1 || Bit1 != 2 || Bit2 != 4 || Bit3 != 8 {
		fmt.Printf("FAIL: iota shift Bit0=%d Bit1=%d Bit2=%d Bit3=%d\n", Bit0, Bit1, Bit2, Bit3)
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
