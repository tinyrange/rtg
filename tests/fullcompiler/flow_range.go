package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Range over slice
	nums := []int{10, 20, 30}
	sum := 0
	for _, v := range nums {
		sum += v
	}
	if sum != 60 {
		fmt.Printf("FAIL: range slice sum=%d\n", sum)
		passed = false
	}

	// Range with index only
	lastIdx := -1
	for i := range nums {
		lastIdx = i
	}
	if lastIdx != 2 {
		fmt.Printf("FAIL: range index only\n")
		passed = false
	}

	// Range over string (with index and value)
	s := "abc"
	count := 0
	for i := range s {
		_ = i
		count++
	}
	if count != 3 {
		fmt.Printf("FAIL: range string count=%d\n", count)
		passed = false
	}

	// Range with blank identifier
	moreNums := []int{1, 2, 3, 4, 5}
	sum = 0
	for _, v := range moreNums {
		sum += v
	}
	if sum != 15 {
		fmt.Printf("FAIL: range blank identifier\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
