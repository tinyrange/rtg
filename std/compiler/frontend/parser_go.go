//go:build !rtg || no_embed_std

package frontend

func ParsePackageFromEmbed(importPath string) *Package {
	return nil
}
