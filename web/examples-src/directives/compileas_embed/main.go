package main

//rtg:compileas id=inner_linux_arm64 target=linux/arm64
func innerLinuxArm64Entry() {
}

//rtg:artifact id=inner_linux_arm64
var innerLinuxArm64 []byte

//rtg:compileas id=inner_windows_386 target=windows/386
func innerWindows386Entry() {
}

//rtg:artifact id=inner_windows_386
var innerWindows386 []byte

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
		println("FAIL: inner linux/arm64 artifact is empty")
		passed = false
	} else if !hasPrefix(innerLinuxArm64, []byte{0x7f, 'E', 'L', 'F'}) {
		println("FAIL: inner linux/arm64 artifact is not ELF")
		passed = false
	}

	if len(innerWindows386) < 2 {
		println("FAIL: inner windows/386 artifact is empty")
		passed = false
	} else if !hasPrefix(innerWindows386, []byte{'M', 'Z'}) {
		println("FAIL: inner windows/386 artifact is not PE/MZ")
		passed = false
	}

	if passed {
		println("PASS")
	} else {
		panic("artifact validation failed")
	}
}
