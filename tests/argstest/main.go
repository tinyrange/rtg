package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Printf("argc = %d\n", len(os.Args))
	i := 0
	for i < len(os.Args) {
		fmt.Printf("arg[%d] = %s\n", i, os.Args[i])
		i = i + 1
	}
	os.Exit(0)
}
