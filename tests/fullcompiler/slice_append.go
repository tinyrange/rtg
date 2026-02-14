package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Append single
	var s []int
	s = append(s, 1)
	s = append(s, 2)
	s = append(s, 3)
	if len(s) != 3 || s[0] != 1 || s[1] != 2 || s[2] != 3 {
		fmt.Printf("FAIL: append single\n")
		passed = false
	}

	// Append multiple
	s2 := []int{10}
	s2 = append(s2, 20, 30, 40)
	if len(s2) != 4 || s2[3] != 40 {
		fmt.Printf("FAIL: append multiple\n")
		passed = false
	}

	// Append slice to slice
	a := []int{1, 2}
	b := []int{3, 4}
	c := append(a, b...)
	if len(c) != 4 || c[2] != 3 || c[3] != 4 {
		fmt.Printf("FAIL: append spread\n")
		passed = false
	}

	// Append grows capacity
	big := make([]int, 0)
	for i := 0; i < 100; i++ {
		big = append(big, i)
	}
	if len(big) != 100 || big[99] != 99 {
		fmt.Printf("FAIL: append grow\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
