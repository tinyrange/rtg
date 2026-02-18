package main

import (
	"fmt"
	_ "os"
)

func main() {
	if len("ok") == 2 {
		fmt.Printf("PASS\n")
		return
	}
	fmt.Printf("FAIL: blank import\n")
}
