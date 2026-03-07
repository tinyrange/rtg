package frontend

import "strings"

// FormatNode renders a parse tree in a stable text format for fixtures.
func FormatNode(root *Node) string {
	if root == nil {
		return ""
	}
	var b strings.Builder
	writeNode(&b, root, 0)
	return b.String()
}

func writeNode(b *strings.Builder, n *Node, depth int) {
	i := 0
	for i < depth {
		b.WriteString("  ")
		i++
	}
	b.WriteString(n.Kind.String())
	if n.Text != "" {
		b.WriteString(": ")
		b.WriteString(n.Text)
	}
	b.WriteByte('\n')
	for _, child := range n.Children {
		writeNode(b, child, depth+1)
	}
}
