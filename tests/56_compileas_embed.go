package main

import (
	"fmt"
	"os"
)

//rtg:compileas id=inner_linux_arm64 target=linux/arm64
func innerLinuxArm64Entry() {
}

//rtg:artifact id=inner_linux_arm64
var innerLinuxArm64 []byte

//rtg:compileas id=inner_windows_amd64 target=windows/amd64
func innerWindowsAmd64Entry() {
}

//rtg:artifact id=inner_windows_amd64
var innerWindowsAmd64 []byte

func hasPrefix(data []byte, prefix []byte) bool {
	if len(data) < len(prefix) {
		return false
	}
	i := 0
	for i < len(prefix) {
		if data[i] != prefix[i] {
			return false
		}
		i++
	}
	return true
}

func main() {
	passed := true

	if len(innerLinuxArm64) < 4 {
		fmt.Printf("FAIL: inner linux/arm64 artifact is empty\n")
		passed = false
	} else if !hasPrefix(innerLinuxArm64, []byte{0x7f, 'E', 'L', 'F'}) {
		fmt.Printf("FAIL: inner linux/arm64 artifact is not ELF\n")
		passed = false
	}

	if len(innerWindowsAmd64) < 2 {
		fmt.Printf("FAIL: inner windows/amd64 artifact is empty\n")
		passed = false
	} else if !hasPrefix(innerWindowsAmd64, []byte{'M', 'Z'}) {
		fmt.Printf("FAIL: inner windows/amd64 artifact is not PE/MZ\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
