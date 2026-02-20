//go:build rtg && no_embed_std

package main

func initEmbeddedStd() {
	// No embedded std — will read from filesystem (or VirtualFS in browser)
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
