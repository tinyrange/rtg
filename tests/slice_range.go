package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Range with index and value
	s := []int{10, 20, 30, 40, 50}
	sum := 0
	idxSum := 0
	for i, v := range s {
		sum += v
		idxSum += i
	}
	if sum != 150 {
		fmt.Printf("FAIL: range sum=%d\n", sum)
		passed = false
	}
	if idxSum != 10 {
		fmt.Printf("FAIL: range idx sum=%d\n", idxSum)
		passed = false
	}

	// Range over empty slice
	empty := []int{}
	count := 0
	for range empty {
		count++
	}
	if count != 0 {
		fmt.Printf("FAIL: range empty\n")
		passed = false
	}

	// Range collecting values
	src := []int{1, 2, 3}
	dst := make([]int, 0)
	for _, v := range src {
		dst = append(dst, v*2)
	}
	if len(dst) != 3 || dst[0] != 2 || dst[1] != 4 || dst[2] != 6 {
		fmt.Printf("FAIL: range collect\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
