package main

import (
	"fmt"
	"os"
	"sort"
)

func main() {
	var names []string
	names = append(names, "cherry")
	names = append(names, "apple")
	names = append(names, "banana")
	names = append(names, "date")

	sort.Strings(names)

	if names[0] != "apple" || names[1] != "banana" || names[2] != "cherry" || names[3] != "date" {
		fmt.Fprintf(os.Stderr, "FAIL: got %s %s %s %s\n", names[0], names[1], names[2], names[3])
		os.Exit(1)
	}
	fmt.Printf("PASS sort.Strings: %s %s %s %s\n", names[0], names[1], names[2], names[3])
	fmt.Printf("All sort tests passed!\n")
}
