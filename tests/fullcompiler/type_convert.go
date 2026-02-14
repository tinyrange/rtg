package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// int to byte
	x := 65
	b := byte(x)
	if b != 65 {
		fmt.Printf("FAIL: int to byte\n")
		passed = false
	}

	// byte to int
	var c byte = 'A'
	n := int(c)
	if n != 65 {
		fmt.Printf("FAIL: byte to int\n")
		passed = false
	}

	// int32 conversion
	var i32 int32 = int32(1000)
	if i32 != 1000 {
		fmt.Printf("FAIL: int32\n")
		passed = false
	}

	// uint conversion
	var u uint = uint(42)
	if u != 42 {
		fmt.Printf("FAIL: uint\n")
		passed = false
	}

	// string to []byte
	s := "hello"
	bs := []byte(s)
	if len(bs) != 5 || bs[0] != 'h' {
		fmt.Printf("FAIL: string to bytes\n")
		passed = false
	}

	// []byte to string
	s2 := string(bs)
	if s2 != "hello" {
		fmt.Printf("FAIL: bytes to string\n")
		passed = false
	}

	// Byte truncation
	big := 256
	small := byte(big)
	if small != 0 {
		fmt.Printf("FAIL: byte truncation\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
