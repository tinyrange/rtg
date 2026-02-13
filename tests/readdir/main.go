package main

import (
	"fmt"
	"os"
)

func isGoFile(name string) bool {
	if len(name) < 4 {
		return false
	}
	return name[len(name)-3:len(name)] == ".go"
}

func main() {
	entries, err := os.ReadDir(".")
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("entries: %d\n", len(entries))
	for _, entry := range entries {
		name := entry.Name()
		fmt.Printf("  name: %s\n", name)
		if isGoFile(name) {
			fmt.Printf("  -> GO FILE\n")
		}
	}
}
