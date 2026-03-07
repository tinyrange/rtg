package main

import (
	"fmt"
	"os"
)

type MyInt = int

type AliasString = string

func main() {
	passed := true

	var x MyInt = 7
	if x+1 != 8 {
		fmt.Printf("FAIL: int alias arithmetic\n")
		passed = false
	}

	var s AliasString = "rtg"
	if len(s) != 3 {
		fmt.Printf("FAIL: string alias length\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
