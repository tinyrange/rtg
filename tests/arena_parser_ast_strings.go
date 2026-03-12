package main

import (
	"fmt"
	"os"
	"strings"

	frontend "j5.nz/rtg/std/compiler/frontend/go"
)

func findFirstStringLit(node *frontend.Node) *frontend.Node {
	if node == nil {
		return nil
	}
	if node.Kind == frontend.NStringLit {
		return node
	}
	if found := findFirstStringLit(node.X); found != nil {
		return found
	}
	if found := findFirstStringLit(node.Y); found != nil {
		return found
	}
	if found := findFirstStringLit(node.Body); found != nil {
		return found
	}
	if found := findFirstStringLit(node.Type); found != nil {
		return found
	}
	i := 0
	for i < len(node.Nodes) {
		if found := findFirstStringLit(node.Nodes[i]); found != nil {
			return found
		}
		i++
	}
	return nil
}

func parseStringLiteralNode(src string) *frontend.Node {
	file := frontend.ParseSource("arena_parser_ast_strings.go", src)
	if file == nil {
		return nil
	}
	return findFirstStringLit(file)
}

func parseTrackedStringLiteralNode() *frontend.Node {
	src := strings.Join([]string{
		"package sample",
		"func f() string {",
		"  if true {",
		"    return \"alpha\\\\nbeta\\\\tgamma\"",
		"  }",
		"  return \"fallback\"",
		"}",
	}, "\n")
	return parseStringLiteralNode(src)
}

func churnParses(rounds int) {
	src := strings.Join([]string{
		"package churn",
		"func g() string {",
		"  return \"0123456789abcdefghijklmnopqrstuvwxyz\"",
		"}",
	}, "\n")
	i := 0
	for i < rounds {
		if frontend.ParseSource("arena_parser_ast_churn.go", src) == nil {
			fmt.Printf("FAIL: churn parse returned nil at round %d\n", i)
			os.Exit(1)
		}
		i++
	}
}

func main() {
	want := "alpha\\\\nbeta\\\\tgamma"
	node := parseTrackedStringLiteralNode()
	if node == nil {
		fmt.Printf("FAIL: string literal node not found\n")
		os.Exit(1)
	}
	if node.Name != want {
		fmt.Printf("FAIL: parse produced %q want %q\n", node.Name, want)
		os.Exit(1)
	}

	churnParses(256)

	if node.Name != want {
		fmt.Printf("FAIL: string literal changed after churn got %q want %q\n", node.Name, want)
		os.Exit(1)
	}

	emptyNode := parseStringLiteralNode(strings.Join([]string{
		"package sample",
		"func empty() string {",
		"  return \"\"",
		"}",
	}, "\n"))
	if emptyNode == nil {
		fmt.Printf("FAIL: empty string literal node not found\n")
		os.Exit(1)
	}
	if emptyNode.Name != "" {
		fmt.Printf("FAIL: empty string literal produced %q want empty\n", emptyNode.Name)
		os.Exit(1)
	}

	fmt.Printf("PASS\n")
}
