package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	inner := map[string]int{"x": 7}
	outer := map[int]map[string]int{1: inner}

	sum := 0
	count := 0
	for _, v := range outer {
		sum += v["x"]
		count++
	}

	if count != 1 {
		fmt.Printf("FAIL: range count=%d\n", count)
		passed = false
	}
	if sum != 7 {
		fmt.Printf("FAIL: nested map value=%d\n", sum)
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
