package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func assert(name string, got string, want string) {
	if got != want {
		fmt.Fprintf(os.Stderr, "FAIL %s: got %q want %q\n", name, got, want)
		os.Exit(1)
	}
	fmt.Printf("PASS %s\n", name)
}

func main() {
	assert("Join basic", filepath.Join("a", "b"), "a/b")
	assert("Join trailing slash", filepath.Join("a/", "b"), "a/b")
	assert("Join leading slash", filepath.Join("a", "/b"), "a/b")
	assert("Join empty a", filepath.Join("", "b"), "b")
	assert("Join empty b", filepath.Join("a", ""), "a")

	assert("Dir basic", filepath.Dir("a/b/c"), "a/b")
	assert("Dir root", filepath.Dir("/a"), "/")
	assert("Dir no slash", filepath.Dir("file"), ".")

	assert("Base basic", filepath.Base("a/b/c"), "c")
	assert("Base trailing slash", filepath.Base("a/b/c/"), "c")
	assert("Base no slash", filepath.Base("file"), "file")

	fmt.Printf("All filepath tests passed!\n")
}
