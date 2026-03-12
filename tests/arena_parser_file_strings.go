package main

import (
	"fmt"
	"os"
	"strings"

	frontend "j5.nz/rtg/std/compiler/frontend/go"
)

func findFileStringLit(node *frontend.Node) *frontend.Node {
	if node == nil {
		return nil
	}
	if node.Kind == frontend.NStringLit {
		return node
	}
	if found := findFileStringLit(node.X); found != nil {
		return found
	}
	if found := findFileStringLit(node.Y); found != nil {
		return found
	}
	if found := findFileStringLit(node.Body); found != nil {
		return found
	}
	if found := findFileStringLit(node.Type); found != nil {
		return found
	}
	i := 0
	for i < len(node.Nodes) {
		if found := findFileStringLit(node.Nodes[i]); found != nil {
			return found
		}
		i++
	}
	return nil
}

func mustWrite(path string, data string) {
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		fmt.Printf("FAIL: write %s: %v\n", path, err)
		os.Exit(1)
	}
}

func parseTrackedFile(path string) *frontend.Node {
	file := frontend.ParseFile(path)
	if file == nil {
		fmt.Printf("FAIL: ParseFile(%s) returned nil\n", path)
		os.Exit(1)
	}
	node := findFileStringLit(file)
	if node == nil {
		fmt.Printf("FAIL: ParseFile(%s) did not produce a string literal\n", path)
		os.Exit(1)
	}
	return node
}

func parseFileString(path string) *frontend.Node {
	file := frontend.ParseFile(path)
	if file == nil {
		fmt.Printf("FAIL: ParseFile(%s) returned nil\n", path)
		os.Exit(1)
	}
	return findFileStringLit(file)
}

func churnFileParses(path string, rounds int) {
	i := 0
	for i < rounds {
		file := frontend.ParseFile(path)
		if file == nil {
			fmt.Printf("FAIL: churn ParseFile round %d returned nil\n", i)
			os.Exit(1)
		}
		i++
	}
}

func main() {
	trackedPath := "build/test_arena_parser_file_strings_input.go"
	churnPath := "build/test_arena_parser_file_strings_churn.go"
	trackedSource := strings.Join([]string{
		"package sample",
		"func f() string {",
		"  return \"tracked\\\\nstring\\\\tvalue\"",
		"}",
	}, "\n")
	churnSource := strings.Join([]string{
		"package churn",
		"func g() string {",
		"  return \"0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ\"",
		"}",
	}, "\n")
	mustWrite(trackedPath, trackedSource)
	mustWrite(churnPath, churnSource)

	want := "tracked\\\\nstring\\\\tvalue"
	node := parseTrackedFile(trackedPath)
	if node.Name != want {
		fmt.Printf("FAIL: ParseFile produced %q want %q\n", node.Name, want)
		os.Exit(1)
	}

	churnFileParses(churnPath, 256)

	if node.Name != want {
		fmt.Printf("FAIL: ParseFile string changed after churn got %q want %q\n", node.Name, want)
		os.Exit(1)
	}

	emptyPath := "build/test_arena_parser_file_empty_input.go"
	mustWrite(emptyPath, strings.Join([]string{
		"package sample",
		"func empty() string {",
		"  return \"\"",
		"}",
	}, "\n"))
	emptyNode := parseFileString(emptyPath)
	if emptyNode == nil {
		fmt.Printf("FAIL: ParseFile empty string literal node not found\n")
		os.Exit(1)
	}
	if emptyNode.Name != "" {
		fmt.Printf("FAIL: ParseFile empty string produced %q want empty\n", emptyNode.Name)
		os.Exit(1)
	}

	fmt.Printf("PASS\n")
}
