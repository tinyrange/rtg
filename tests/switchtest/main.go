package main

import (
	"fmt"
	"os"
)

func main() {
	args := os.Args
	fmt.Printf("argc: %d\n", len(args))
	i := 0
	for i < len(args) {
		fmt.Printf("arg[%d]: %q\n", i, args[i])
		i++
	}

	if len(args) > 1 {
		arg := args[1]
		fmt.Printf("testing switch on %q\n", arg)
		switch arg {
		case "--help":
			fmt.Printf("matched --help\n")
		case "--list":
			fmt.Printf("matched --list\n")
		default:
			fmt.Printf("no match\n")
		}
	}
}
