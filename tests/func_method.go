package main

import (
	"fmt"
	"os"
)

type Point struct {
	X int
	Y int
}

func (p Point) Sum() int {
	return p.X + p.Y
}

func (p *Point) Scale(factor int) {
	p.X = p.X * factor
	p.Y = p.Y * factor
}

type Counter struct {
	val int
}

func (c *Counter) Inc() {
	c.val = c.val + 1
}

func (c *Counter) Get() int {
	return c.val
}

func main() {
	passed := true

	// Value receiver
	pt := Point{X: 3, Y: 4}
	if pt.Sum() != 7 {
		fmt.Printf("FAIL: value method\n")
		passed = false
	}

	// Pointer receiver
	pt.Scale(2)
	if pt.X != 6 || pt.Y != 8 {
		fmt.Printf("FAIL: pointer method\n")
		passed = false
	}

	// Method on pointer
	c := &Counter{val: 0}
	c.Inc()
	c.Inc()
	c.Inc()
	if c.Get() != 3 {
		fmt.Printf("FAIL: method on pointer get=%d\n", c.Get())
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
