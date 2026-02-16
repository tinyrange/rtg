package main

import (
	"fmt"
	"os"
)

type Address struct {
	City    string
	ZipCode int
}

type Person struct {
	Name    string
	Age     int
	Address Address
}

func main() {
	passed := true

	p := Person{
		Name: "Bob",
		Age:  25,
		Address: Address{
			City:    "NYC",
			ZipCode: 10001,
		},
	}

	if p.Name != "Bob" {
		fmt.Printf("FAIL: outer name\n")
		passed = false
	}
	if p.Address.City != "NYC" {
		fmt.Printf("FAIL: nested city\n")
		passed = false
	}
	if p.Address.ZipCode != 10001 {
		fmt.Printf("FAIL: nested zip\n")
		passed = false
	}

	// Modify nested
	p.Address.City = "LA"
	if p.Address.City != "LA" {
		fmt.Printf("FAIL: modify nested\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
