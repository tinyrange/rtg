package main

import (
	"fmt"
	"os"
)

type Point struct {
	X int
	Y int
}

func main() {
	passed := true

	// Slice of struct literals
	points := []Point{
		{X: 1, Y: 2},
		{X: 3, Y: 4},
		{X: 5, Y: 6},
	}
	if len(points) != 3 || points[1].X != 3 {
		fmt.Printf("FAIL: slice of struct\n")
		passed = false
	}

	// Nested slice literal
	matrix := [][]int{
		{1, 2, 3},
		{4, 5, 6},
	}
	if matrix[1][2] != 6 {
		fmt.Printf("FAIL: nested slice\n")
		passed = false
	}

	// Map literal with struct values
	m := map[string]Point{
		"origin": {X: 0, Y: 0},
		"unit":   {X: 1, Y: 1},
	}
	if m["origin"].X != 0 || m["unit"].Y != 1 {
		fmt.Printf("FAIL: map struct literal\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
