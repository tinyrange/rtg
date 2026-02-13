package main

import (
	"fmt"
	"os"
)

func main() {
	name := os.Args[0]
	fmt.Printf("name: %s\n", name)
	fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n  %s build\n  %s run\n  %s test\n  %s help\n  %s version\n", name, name, name, name, name, name)
	fmt.Printf("done\n")
}
