package main

import (
	"fmt"
	"os"
)

type Item struct {
	Name  string
	Value int
}

func main() {
	passed := true

	// Slice of slices
	matrix := [][]int{
		{1, 2, 3},
		{4, 5, 6},
		{7, 8, 9},
	}
	if matrix[1][1] != 5 {
		fmt.Printf("FAIL: matrix\n")
		passed = false
	}

	// Slice of structs
	items := []Item{
		{Name: "a", Value: 1},
		{Name: "b", Value: 2},
	}
	if items[0].Name != "a" || items[1].Value != 2 {
		fmt.Printf("FAIL: slice of structs\n")
		passed = false
	}

	// Slice of pointers
	x := 10
	y := 20
	ptrs := []*int{&x, &y}
	if *ptrs[0] != 10 || *ptrs[1] != 20 {
		fmt.Printf("FAIL: slice of pointers\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
