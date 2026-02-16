package main

import (
	"fmt"
	"os"
)

type Point struct {
	X int
	Y int
}

type Person struct {
	Name string
	Age  int
}

func main() {
	passed := true

	// Positional literal
	p1 := Point{3, 4}
	if p1.X != 3 || p1.Y != 4 {
		fmt.Printf("FAIL: positional literal\n")
		passed = false
	}

	// Named literal
	p2 := Point{X: 10, Y: 20}
	if p2.X != 10 || p2.Y != 20 {
		fmt.Printf("FAIL: named literal\n")
		passed = false
	}

	// Field assignment
	p2.X = 100
	if p2.X != 100 {
		fmt.Printf("FAIL: field assign\n")
		passed = false
	}

	// Zero value
	var p3 Point
	if p3.X != 0 || p3.Y != 0 {
		fmt.Printf("FAIL: zero value\n")
		passed = false
	}

	// String fields
	per := Person{Name: "Alice", Age: 30}
	if per.Name != "Alice" || per.Age != 30 {
		fmt.Printf("FAIL: string field\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
