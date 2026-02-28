package main

import (
	"fmt"
	"os"
)

type Stringer interface {
	String() string
}

type Dog struct {
	Name string
}

func (d Dog) String() string {
	return "Dog: " + d.Name
}

type Cat struct {
	Name string
}

func (c Cat) String() string {
	return "Cat: " + c.Name
}

func describe(s Stringer) string {
	return s.String()
}

func main() {
	passed := true

	d := Dog{Name: "Rex"}
	c := Cat{Name: "Whiskers"}

	if describe(d) != "Dog: Rex" {
		fmt.Printf("FAIL: dog string\n")
		passed = false
	}
	if describe(c) != "Cat: Whiskers" {
		fmt.Printf("FAIL: cat string\n")
		passed = false
	}

	// Interface variable
	var s Stringer = d
	if s.String() != "Dog: Rex" {
		fmt.Printf("FAIL: iface var\n")
		passed = false
	}

	// Reassign interface
	s = c
	if s.String() != "Cat: Whiskers" {
		fmt.Printf("FAIL: iface reassign\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
