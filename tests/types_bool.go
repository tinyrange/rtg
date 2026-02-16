package main

import (
	"fmt"
	"os"
)

func sideEffect(val *int) bool {
	*val = *val + 1
	return true
}

func main() {
	passed := true

	// Basic bool
	t := true
	f := false
	if !t {
		fmt.Printf("FAIL: true is not true\n")
		passed = false
	}
	if f {
		fmt.Printf("FAIL: false is not false\n")
		passed = false
	}

	// AND
	if !(t && t) {
		fmt.Printf("FAIL: true && true\n")
		passed = false
	}
	if t && f {
		fmt.Printf("FAIL: true && false\n")
		passed = false
	}

	// OR
	if !(t || f) {
		fmt.Printf("FAIL: true || false\n")
		passed = false
	}
	if f || f {
		fmt.Printf("FAIL: false || false\n")
		passed = false
	}

	// NOT
	if !(!f) == false {
	} else {
		// This is just to test !
	}
	if !!t != true {
		fmt.Printf("FAIL: !!true\n")
		passed = false
	}

	// Short-circuit: && should not eval right side if left is false
	counter := 0
	if false && sideEffect(&counter) {
		fmt.Printf("FAIL: short-circuit &&\n")
		passed = false
	}
	if counter != 0 {
		fmt.Printf("FAIL: && short-circuit didn't skip right side\n")
		passed = false
	}

	// Short-circuit: || should not eval right side if left is true
	counter = 0
	if true || sideEffect(&counter) {
	}
	if counter != 0 {
		fmt.Printf("FAIL: || short-circuit didn't skip right side\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
