package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Slicing
	s := []int{0, 1, 2, 3, 4, 5}
	sub := s[1:4]
	if len(sub) != 3 || sub[0] != 1 || sub[2] != 3 {
		fmt.Printf("FAIL: slicing\n")
		passed = false
	}

	// Slicing from start
	head := s[:3]
	if len(head) != 3 || head[2] != 2 {
		fmt.Printf("FAIL: slice head\n")
		passed = false
	}

	// Slicing to end
	tail := s[3:]
	if len(tail) != 3 || tail[0] != 3 {
		fmt.Printf("FAIL: slice tail\n")
		passed = false
	}

	// Copy
	src := []int{1, 2, 3}
	dst := make([]int, 3)
	n := copy(dst, src)
	if n != 3 || dst[0] != 1 || dst[2] != 3 {
		fmt.Printf("FAIL: copy\n")
		passed = false
	}

	// Nil slice
	var nilSlice []int
	if len(nilSlice) != 0 {
		fmt.Printf("FAIL: nil slice len\n")
		passed = false
	}

	// Append to nil
	nilSlice = append(nilSlice, 42)
	if len(nilSlice) != 1 || nilSlice[0] != 42 {
		fmt.Printf("FAIL: append nil\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
