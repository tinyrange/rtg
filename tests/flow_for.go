package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// For with condition
	sum := 0
	i := 0
	for i < 10 {
		sum = sum + i
		i = i + 1
	}
	if sum != 45 {
		fmt.Printf("FAIL: for condition sum=%d\n", sum)
		passed = false
	}

	// Three-clause for
	sum = 0
	for j := 0; j < 10; j++ {
		sum = sum + j
	}
	if sum != 45 {
		fmt.Printf("FAIL: three-clause for\n")
		passed = false
	}

	// Infinite for + break
	count := 0
	for {
		count++
		if count >= 5 {
			break
		}
	}
	if count != 5 {
		fmt.Printf("FAIL: infinite for + break\n")
		passed = false
	}

	// Nested loops
	total := 0
	for a := 0; a < 3; a++ {
		for b := 0; b < 3; b++ {
			total++
		}
	}
	if total != 9 {
		fmt.Printf("FAIL: nested loops\n")
		passed = false
	}

	// For with decrement
	val := 10
	for val > 0 {
		val = val - 1
	}
	if val != 0 {
		fmt.Printf("FAIL: for decrement\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
