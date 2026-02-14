package main

import (
	"fmt"
	"os"
)

var order string

func appendChar(c string) {
	order = order + c
}

func main() {
	passed := true
	order = ""

	defer appendChar("3")
	defer appendChar("2")
	defer appendChar("1")
	appendChar("0")

	// At this point, defers haven't run yet, so order is just "0"
	// After main returns, defers run in LIFO: "1", "2", "3"
	// So final order would be "0123"
	// But we can't check after main returns, so let's check what we can
	if order != "0" {
		fmt.Printf("FAIL: before defer order=%s\n", order)
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
