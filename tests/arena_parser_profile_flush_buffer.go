package main

import (
	"fmt"
	"os"

	frontend "j5.nz/rtg/std/compiler/frontend/go"
)

type nodeSnapshot struct {
	node *frontend.Node
	kind frontend.NodeKind
	name string
}

func collectSubtree(out *[]nodeSnapshot, node *frontend.Node) {
	if node == nil {
		return
	}
	*out = append(*out, nodeSnapshot{node: node, kind: node.Kind, name: node.Name})
	collectSubtree(out, node.X)
	collectSubtree(out, node.Y)
	collectSubtree(out, node.Body)
	collectSubtree(out, node.Type)
	i := 0
	for i < len(node.Nodes) {
		collectSubtree(out, node.Nodes[i])
		i++
	}
}

func mustParse(path string) *frontend.Node {
	file := frontend.ParseFile(path)
	if file == nil {
		fmt.Printf("FAIL: ParseFile(%s) returned nil\n", path)
		os.Exit(1)
	}
	return file
}

func findFunc(file *frontend.Node, name string) *frontend.Node {
	if file == nil {
		return nil
	}
	i := 0
	for i < len(file.Nodes) {
		node := file.Nodes[i]
		if node != nil && node.Kind == frontend.NFunc && node.Name == name {
			return node
		}
		i++
	}
	return nil
}

func main() {
	file := mustParse("std/runtime/runtime_profile.go")
	fn := findFunc(file, "profileFlushBuffer")
	if fn == nil {
		fmt.Printf("FAIL: profileFlushBuffer not found\n")
		os.Exit(1)
	}
	tracked := []nodeSnapshot{}
	collectSubtree(&tracked, fn)
	if len(tracked) == 0 {
		fmt.Printf("FAIL: no nodes tracked\n")
		os.Exit(1)
	}

	churnFiles := []string{
		"std/compiler/frontend/go/compiler.go",
		"std/compiler/frontend/go/parser.go",
		"std/compiler/frontend/go/frontend.go",
		"std/runtime/runtime.go",
		"std/runtime/runtime_profile.go",
		"std/compiler/main.go",
	}
	round := 0
	for round < 4 {
		i := 0
		for i < len(churnFiles) {
			mustParse(churnFiles[i])
			i++
		}
		round++
	}

	i := 0
	for i < len(tracked) {
		snap := tracked[i]
		if snap.node == nil {
			fmt.Printf("FAIL: tracked node became nil at idx=%d\n", i)
			os.Exit(1)
		}
		if snap.node.Kind != snap.kind || snap.node.Name != snap.name {
			fmt.Printf("FAIL: tracked node changed at idx=%d want(kind=%d name=%q) got(kind=%d name=%q)\n", i, snap.kind, snap.name, snap.node.Kind, snap.node.Name)
			os.Exit(1)
		}
		i++
	}

	fmt.Printf("PASS\n")
}
