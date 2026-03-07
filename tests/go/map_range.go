package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	m := map[string]int{
		"a": 1,
		"b": 2,
		"c": 3,
	}

	// Range over map - collect all values
	sum := 0
	count := 0
	for _, v := range m {
		sum += v
		count++
	}
	if count != 3 {
		fmt.Printf("FAIL: range count=%d\n", count)
		passed = false
	}
	if sum != 6 {
		fmt.Printf("FAIL: range sum=%d\n", sum)
		passed = false
	}

	// Range over keys
	keys := make([]string, 0)
	for k := range m {
		keys = append(keys, k)
	}
	if len(keys) != 3 {
		fmt.Printf("FAIL: range keys=%d\n", len(keys))
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
