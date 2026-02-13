package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	// Test Command + Run
	cmd := exec.Command("/bin/echo", "hello", "world")
	cmd.Stdout = os.Stdout
	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL Run: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("PASS Run\n")

	// Test Command + Output
	cmd2 := exec.Command("/bin/echo", "captured")
	out, err := cmd2.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL Output: %v\n", err)
		os.Exit(1)
	}
	result := strings.TrimSpace(string(out))
	if result != "captured" {
		fmt.Fprintf(os.Stderr, "FAIL Output: got %q want %q\n", result, "captured")
		os.Exit(1)
	}
	fmt.Printf("PASS Output: %s\n", result)

	// Test exit code
	cmd3 := exec.Command("/bin/false")
	err = cmd3.Run()
	if err == nil {
		fmt.Fprintf(os.Stderr, "FAIL ExitError: expected error\n")
		os.Exit(1)
	}
	fmt.Printf("PASS ExitError: %s\n", err)

	fmt.Printf("All exec tests passed!\n")
}
