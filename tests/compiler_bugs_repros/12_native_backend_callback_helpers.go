package main

func compileFunc(fn func()) {
	fn() // Repro shape for unresolved calls through callback-style helpers.
}

func emitTostringHelper(fn func()) {
	fn()
}

func patchAt(fn func()) {
	fn()
}

func main() {
	compileFunc(func() {})
	emitTostringHelper(func() {})
	patchAt(func() {})
}
