//go:build !rtg

package main

func initEmbeddedStd() {
	// No embedded std in Go bootstrap mode — will read from disk
}

func hasEmbeddedStd() bool {
	return false
}

func walkEmbedFromFS(embedDir string) ([]string, []string) {
	return nil, nil
}

func parsePackageFromEmbed(importPath string) *Package {
	return nil
}
