package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"j5.nz/rtg/std/compiler/binary"
	"j5.nz/rtg/std/compiler/common"
	front "j5.nz/rtg/std/compiler/frontend/go"
)

type summaryPackage struct {
	Name  string
	Files []*front.Node
}

func main() {
	repoRoot, err := os.Getwd()
	if err != nil {
		fail("getwd: %v", err)
	}
	target := &common.Target{
		GOOS:      "linux",
		GOARCH:    "amd64",
		PtrSize:   8,
		WordSize:  8,
		Strict:    true,
		BuildTags: []string{"linux", "amd64", "rtg", "no_embed_std", "regen_prebuilt_stdlib"},
	}

	mod := front.ResolveModule(target, repoRoot, []string{"compiler"})
	compilerIR, compilerErrs := front.CompileModule(*target, mod)
	if len(compilerErrs) > 0 {
		fail("compile selfhost IR: %s", strings.Join(compilerErrs, "; "))
	}
	prebuiltPaths := collectPrebuiltPaths(mod)
	if len(prebuiltPaths) == 0 {
		fail("no prebuilt packages discovered")
	}

	summaries := make(map[string]*summaryPackage, len(prebuiltPaths))
	pkgs := make(map[string]*front.Package, len(prebuiltPaths)+1)
	for _, path := range prebuiltPaths {
		pkg := mod.Packages[path]
		summaries[path] = &summaryPackage{
			Name:  pkg.Name,
			Files: stripFiles(pkg.Files),
		}
		pkgs[path] = pkg
	}

	mainFile := &front.Node{Kind: front.NFile, Name: "main"}
	for _, path := range prebuiltPaths {
		mainFile.Nodes = append(mainFile.Nodes, &front.Node{Kind: front.NImport, Name: path})
	}
	mainPkg := &front.Package{
		Name:    "main",
		Path:    "main",
		Dir:     "main",
		Files:   []*front.Node{mainFile},
		Imports: append([]string{}, prebuiltPaths...),
		Symbols: make(map[string]*front.Symbol),
	}
	pkgs["main"] = mainPkg

	prebuiltMod := &front.Module{
		BaseDir:  repoRoot,
		Packages: pkgs,
		Order:    append(append([]string{}, prebuiltPaths...), "main"),
		Entry:    mainPkg,
	}
	irmod, errs := front.CompileModule(*target, prebuiltMod)
	if len(errs) > 0 {
		fail("compile prebuilt IR: %s", strings.Join(errs, "; "))
	}

	summaryRoot := filepath.Join(repoRoot, "std/compiler/frontend/go/prebuilt_stdlib_summaries")
	if err := os.RemoveAll(summaryRoot); err != nil {
		fail("remove old summaries: %v", err)
	}
	for _, path := range prebuiltPaths {
		outPath := filepath.Join(summaryRoot, filepath.FromSlash(path)+".ps")
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			fail("mkdir %s: %v", outPath, err)
		}
		if err := os.WriteFile(outPath, []byte(encodeSummary(summaries[path])), 0644); err != nil {
			fail("write summary %s: %v", outPath, err)
		}
	}

	irPath := filepath.Join(repoRoot, "std/compiler/frontend/go/prebuilt_stdlib.irt")
	if err := binary.WriteIRText(irmod, irPath); err != nil {
		fail("write prebuilt IR: %v", err)
	}
	compilerIRPath := filepath.Join(repoRoot, "std/compiler/frontend/go/prebuilt_selfhost_compiler.irt")
	if err := binary.WriteIRText(compilerIR, compilerIRPath); err != nil {
		fail("write prebuilt compiler IR: %v", err)
	}
}

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func collectPrebuiltPaths(mod *front.Module) []string {
	var paths []string
	for _, path := range mod.Order {
		if path == "main" {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func stripFiles(files []*front.Node) []*front.Node {
	out := make([]*front.Node, 0, len(files))
	for _, file := range files {
		out = append(out, stripNode(file))
	}
	return out
}

func stripNode(node *front.Node) *front.Node {
	if node == nil {
		return nil
	}
	out := &front.Node{
		Kind: node.Kind,
		Name: node.Name,
	}
	out.X = stripNode(node.X)
	out.Y = stripNode(node.Y)
	out.Type = stripNode(node.Type)
	if node.Kind != front.NFunc {
		out.Body = stripNode(node.Body)
	}
	if len(node.Nodes) > 0 {
		out.Nodes = make([]*front.Node, 0, len(node.Nodes))
		for _, child := range node.Nodes {
			out.Nodes = append(out.Nodes, stripNode(child))
		}
	}
	return out
}

func encodeSummary(summary *summaryPackage) string {
	ids := make(map[*front.Node]int)
	var nodes []*front.Node
	for _, file := range summary.Files {
		collectNodeIDs(file, ids, &nodes)
	}
	var b strings.Builder
	b.WriteString("pkg\t")
	b.WriteString(encodeHex(summary.Name))
	b.WriteByte('\n')
	b.WriteString("files\t")
	writeNodeIDList(&b, summary.Files, ids)
	b.WriteByte('\n')
	for _, node := range nodes {
		fmt.Fprintf(&b, "n\t%d\t%d\t%s\t%d\t%d\t%d\t%d\t",
			ids[node],
			int(node.Kind),
			encodeHex(node.Name),
			nodeID(ids, node.X),
			nodeID(ids, node.Y),
			nodeID(ids, node.Body),
			nodeID(ids, node.Type))
		writeNodeIDList(&b, node.Nodes, ids)
		b.WriteByte('\n')
	}
	return b.String()
}

func collectNodeIDs(node *front.Node, ids map[*front.Node]int, nodes *[]*front.Node) int {
	if node == nil {
		return 0
	}
	if id, ok := ids[node]; ok {
		return id
	}
	id := len(*nodes) + 1
	ids[node] = id
	*nodes = append(*nodes, node)
	collectNodeIDs(node.X, ids, nodes)
	collectNodeIDs(node.Y, ids, nodes)
	collectNodeIDs(node.Body, ids, nodes)
	collectNodeIDs(node.Type, ids, nodes)
	for _, child := range node.Nodes {
		collectNodeIDs(child, ids, nodes)
	}
	return id
}

func nodeID(ids map[*front.Node]int, node *front.Node) int {
	if node == nil {
		return 0
	}
	return ids[node]
}

func writeNodeIDList(b *strings.Builder, nodes []*front.Node, ids map[*front.Node]int) {
	for i, node := range nodes {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(b, "%d", nodeID(ids, node))
	}
}

func encodeHex(s string) string {
	if s == "" {
		return ""
	}
	const digits = "0123456789abcdef"
	buf := make([]byte, len(s)*2)
	for i := 0; i < len(s); i++ {
		buf[i*2] = digits[s[i]>>4]
		buf[i*2+1] = digits[s[i]&0x0f]
	}
	return string(buf)
}
