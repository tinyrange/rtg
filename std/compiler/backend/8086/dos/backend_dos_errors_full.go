//go:build !no_backend_dos_i386 && !tiny_dos_backend

package dos

import (
	"fmt"
	"os"
)

func reportUnresolvedCalls(unresolved []string) {
	seen := map[string]bool{}
	fmt.Fprintf(os.Stderr, "error: %d unresolved calls:\n", len(unresolved))
	for _, name := range unresolved {
		if !seen[name] {
			fmt.Fprintf(os.Stderr, "  %s\n", name)
			seen[name] = true
		}
	}
}

func errUnresolvedCalls(count int) error {
	return fmt.Errorf("%d unresolved calls", count)
}

func errWriteOutput(err error) error {
	return fmt.Errorf("write output: %v", err)
}

func errCOMTooLarge(total int, max int, text int, rodata int, data int) error {
	return fmt.Errorf("COM image too large: %d bytes (max %d), text=%d rodata=%d data=%d", total, max, text, rodata, data)
}
