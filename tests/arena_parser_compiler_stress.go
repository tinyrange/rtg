package main

import (
	"fmt"
	"os"

	frontend "j5.nz/rtg/std/compiler/frontend/go"
	"runtime"
)

func collectEmptyStringLits(node *frontend.Node, out *[]*frontend.Node) {
	if node == nil {
		return
	}
	if node.Kind == frontend.NStringLit && node.Name == "" {
		*out = append(*out, node)
	}
	collectEmptyStringLits(node.X, out)
	collectEmptyStringLits(node.Y, out)
	collectEmptyStringLits(node.Body, out)
	collectEmptyStringLits(node.Type, out)
	i := 0
	for i < len(node.Nodes) {
		collectEmptyStringLits(node.Nodes[i], out)
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

func main() {
	tracked := []*frontend.Node{}
	collectEmptyStringLits(mustParse("std/runtime/runtime.go"), &tracked)
	if len(tracked) == 0 {
		fmt.Printf("FAIL: no empty string literals found in std/runtime/runtime.go\n")
		os.Exit(1)
	}

	churnFiles := []string{
		"std/compiler/frontend/go/compiler.go",
		"std/compiler/backend/wasm32/backend_wasm32.go",
		"std/compiler/backend/vm/backend_vm.go",
		"std/compiler/backend/c/backend_c.go",
		"std/compiler/frontend/go/parser.go",
		"std/compiler/main.go",
		"std/compiler/frontend/go/frontend.go",
		"std/runtime/runtime.go",
		"std/compiler/binary/ir_text_codec.go",
		"std/compiler/backend/aarch64/backend_aarch64.go",
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
		if tracked[i] == nil || tracked[i].Name != "" {
			gotLen := -1
			gotPtr := uintptr(0)
			gotKind := -1
			gotPos := -1
			if tracked[i] != nil {
				gotLen = len(tracked[i].Name)
				gotPtr = runtime.Stringptr(tracked[i].Name)
				gotKind = int(tracked[i].Kind)
				gotPos = tracked[i].Pos
			}
			fmt.Printf("FAIL: empty string literal changed after compiler churn at idx=%d len=%d ptr=%d kind=%d pos=%d\n", i, gotLen, gotPtr, gotKind, gotPos)
			os.Exit(1)
		}
		i++
	}

	fmt.Printf("PASS\n")
}
