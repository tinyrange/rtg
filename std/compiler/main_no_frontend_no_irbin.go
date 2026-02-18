//go:build no_frontend && !exp_ir_binary

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintf(os.Stderr, "no_frontend build requires experimental IR binary mode; rebuild with -tags exp_ir_binary\n")
	os.Exit(1)
}
