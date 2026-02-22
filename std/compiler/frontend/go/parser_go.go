//go:build no_embed_std

package frontend

func (p *Preprocessor) parsePackageFromEmbed(importPath string) *Package {
	return nil
}
