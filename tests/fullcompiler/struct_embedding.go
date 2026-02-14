package main

import (
	"fmt"
	"os"
)

type Base struct {
	ID int
}

func (b Base) GetID() int {
	return b.ID
}

type Derived struct {
	Base
	Name string
}

func main() {
	passed := true

	d := Derived{
		Base: Base{ID: 42},
		Name: "test",
	}

	// Access embedded field directly
	if d.ID != 42 {
		fmt.Printf("FAIL: embedded field\n")
		passed = false
	}

	// Access through embedded name
	if d.Base.ID != 42 {
		fmt.Printf("FAIL: explicit embedded\n")
		passed = false
	}

	// Promoted method
	if d.GetID() != 42 {
		fmt.Printf("FAIL: promoted method\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
