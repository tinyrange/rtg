package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	var i interface{} = 42

	// Type assertion
	v := i.(int)
	if v != 42 {
		fmt.Printf("FAIL: type assert\n")
		passed = false
	}

	// Comma-ok type assertion
	v2 := 0
	ok := false
	switch i.(type) {
	case int:
		ok = true
		v2 = i.(int)
	}
	if !ok || v2 != 42 {
		fmt.Printf("FAIL: comma ok int\n")
		passed = false
	}

	// Failed assertion with comma-ok
	ok2 := false
	switch i.(type) {
	case string:
		ok2 = true
	}
	if ok2 {
		fmt.Printf("FAIL: comma ok wrong type\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
