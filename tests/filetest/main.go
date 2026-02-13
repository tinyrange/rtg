package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Printf("reading dir...\n")
	entries, err := os.ReadDir("/home/astra/dev/projects/rtg2/testdata/argstest")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: ReadDir failed\n")
		os.Exit(1)
	}
	fmt.Printf("got %d entries\n", len(entries))
	for _, entry := range entries {
		name := entry.Name()
		fmt.Printf("entry: %s isDir=%d\n", name, entry.IsDir())
	}
	fmt.Printf("reading file...\n")
	data, err := os.ReadFile("/home/astra/dev/projects/rtg2/testdata/argstest/main.go")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: ReadFile failed\n")
		os.Exit(1)
	}
	fmt.Printf("file size: %d\n", len(data))
	fmt.Printf("done\n")
	os.Exit(0)
}
