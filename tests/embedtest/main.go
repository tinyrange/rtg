package main

import (
	"embed"
	"fmt"
)

//go:embed testdata
var testFS embed.FS

func main() {
	content := testFS.ReadFile("hello.txt")
	fmt.Println("content:", content)
	fmt.Println("content len:", len(content))
}
