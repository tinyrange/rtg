//go:build !no_embed_std

package frontend

import (
	"j5.nz/rtg/std/compiler/arena"
	"j5.nz/rtg/std/compiler/stdlib"
)

func (p *Preprocessor) parsePackageFromEmbed(importPath string) *Package {
	arena.Enter("frontend.parsePackageFromEmbed")
	defer arena.Leave()
	names := stdlib.ReadDirFromEmbed(importPath)
	if len(names) == 0 {
		return nil
	}

	var goFiles []packageSourceFile
	for _, name := range names {
		if !isGoFile(name) {
			continue
		}
		content := stdlib.ReadFileFromEmbed(importPath + "/" + name)
		if p.shouldIncludeContent(content, name) {
			goFiles = append(goFiles, packageSourceFile{
				name:    name,
				path:    importPath + "/" + name,
				content: content,
			})
		}
	}
	sortPackageSourceFiles(goFiles)
	if len(goFiles) == 0 {
		return nil
	}

	arena.UseParent()
	pkg := &Package{
		Path:    importPath,
		Dir:     importPath,
		Symbols: make(map[string]*Symbol),
	}

	i := 0
	for i < len(goFiles) {
		file := goFiles[i]
		node := parseSource(file.path, file.content)
		if node != nil {
			if pkg.Name == "" {
				pkg.Name = node.Name
			}
			pkg.Files = append(pkg.Files, node)
		}
		i = i + 1
	}

	if len(pkg.Files) == 0 {
		arena.Restore()
		return nil
	}

	pkg.Imports = collectImports(pkg)
	arena.Restore()
	return pkg
}
