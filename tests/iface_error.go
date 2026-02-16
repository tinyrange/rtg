package main

import (
	"fmt"
	"os"
)

type MyError struct {
	msg string
}

func (e *MyError) Error() string {
	return e.msg
}

func divide(a, b int) (int, error) {
	if b == 0 {
		return 0, &MyError{msg: "division by zero"}
	}
	return a / b, nil
}

func main() {
	passed := true

	// Success case
	v, err := divide(10, 2)
	if err != nil {
		fmt.Printf("FAIL: divide ok err\n")
		passed = false
	}
	if v != 5 {
		fmt.Printf("FAIL: divide ok val\n")
		passed = false
	}

	// Error case
	_, err = divide(10, 0)
	if err == nil {
		fmt.Printf("FAIL: divide err nil\n")
		passed = false
	}
	if err.Error() != "division by zero" {
		fmt.Printf("FAIL: error msg=%s\n", err.Error())
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
