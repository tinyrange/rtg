package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Break in for
	count := 0
	for i := 0; i < 100; i++ {
		if i == 5 {
			break
		}
		count++
	}
	if count != 5 {
		fmt.Printf("FAIL: break count=%d\n", count)
		passed = false
	}

	// Continue in for
	sum := 0
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			continue
		}
		sum += i
	}
	// sum = 1+3+5+7+9 = 25
	if sum != 25 {
		fmt.Printf("FAIL: continue sum=%d\n", sum)
		passed = false
	}

	// Break in nested for
	outerCount := 0
	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			if j == 2 {
				break
			}
		}
		outerCount++
	}
	if outerCount != 5 {
		fmt.Printf("FAIL: nested break\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
