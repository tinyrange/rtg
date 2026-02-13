//go:build rtg && !no_embed_std

package main

import "embed"

//go:embed ..
var embeddedStd embed.FS

func initEmbeddedStd() {
	// embeddedStd is populated by init$globals via compileEmbedInit
}

func hasEmbeddedStd() bool {
	return true
}

func walkEmbedFromFS(embedDir string) ([]string, []string) {
	return embeddedStd.WalkDir(embedDir)
}

func parsePackageFromEmbed(importPath string) *Package {
	// List files in the embedded std directory for this import path
	files := embeddedStd.ReadDir(importPath)
	if len(files) == 0 {
		return nil
	}

	// Filter and sort .go files
	var goFiles []string
	i := 0
	for i < len(files) {
		name := files[i]
		if isGoFile(name) {
			content := embeddedStd.ReadFile(importPath + "/" + name)
			if shouldIncludeContent(content, name) {
				goFiles = append(goFiles, name)
			}
		}
		i = i + 1
	}
	sortStrings(goFiles)

	pkg := &Package{
		Path:    importPath,
		Dir:     importPath,
		Symbols: make(map[string]*Symbol),
	}

	i = 0
	for i < len(goFiles) {
		name := goFiles[i]
		content := embeddedStd.ReadFile(importPath + "/" + name)
		node := parseSource(importPath+"/"+name, content)
		if node != nil {
			if pkg.Name == "" {
				pkg.Name = node.Name
			}
			pkg.Files = append(pkg.Files, node)
		}
		i = i + 1
	}

	if len(pkg.Files) == 0 {
		return nil
	}

	pkg.Imports = collectImports(pkg)
	return pkg
}
