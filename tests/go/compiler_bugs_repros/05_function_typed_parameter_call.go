package main

func wrapper(fn func()) {
	fn() // Selfhost unresolved call repro shape: unresolved calls: fn.
}

func main() {
	wrapper(func() {})
}
