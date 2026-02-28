package main

var globals = []string{
	"runtime/runtime_linux_amd64.go",
	"runtime/runtime_wasi_wasm32.go",
}

func main() {
	_ = globals // Repro scaffold for cross-stage global-init drift investigation.
}
