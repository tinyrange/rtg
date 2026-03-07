package main

import (
	"fmt"
	"os"
)

type Info struct {
	Name string
	Age  int
}

func main() {
	passed := true

	// map[int]string
	m1 := make(map[int]string)
	m1[1] = "one"
	m1[2] = "two"
	if m1[1] != "one" {
		fmt.Printf("FAIL: map[int]string\n")
		passed = false
	}

	// map[string]struct
	m2 := make(map[string]Info)
	m2["alice"] = Info{Name: "Alice", Age: 30}
	if m2["alice"].Name != "Alice" || m2["alice"].Age != 30 {
		fmt.Printf("FAIL: map struct val\n")
		passed = false
	}

	// map with pointer values
	x := 42
	m3 := make(map[string]*int)
	m3["ptr"] = &x
	if *m3["ptr"] != 42 {
		fmt.Printf("FAIL: map pointer val\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
