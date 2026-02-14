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

	// Pointer to struct
	p := Point{X: 1, Y: 2}
	pp := &p
	if pp.X != 1 || pp.Y != 2 {
		fmt.Printf("FAIL: ptr deref\n")
		passed = false
	}

	// Modify through pointer (auto-deref)
	pp.X = 100
	if p.X != 100 {
		fmt.Printf("FAIL: ptr modify\n")
		passed = false
	}

	// &Struct{} syntax
	pp2 := &Point{X: 5, Y: 6}
	if pp2.X != 5 || pp2.Y != 6 {
		fmt.Printf("FAIL: &Struct{}\n")
		passed = false
	}

	// Nil struct pointer
	var pp3 *Point
	if pp3 != nil {
		fmt.Printf("FAIL: nil struct ptr\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
