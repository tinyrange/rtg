package main

type FrontendOptions struct {
	Strict bool
}

func withOptions(opts *FrontendOptions, strict bool, fn func()) {
	prev := opts.Strict
	opts.Strict = strict
	fn()
	opts.Strict = prev
}

func main() {
	opts := &FrontendOptions{}
	withOptions(opts, true, func() {})
	withOptions(opts, false, func() {}) // Repeated apply/restore helper drift repro shape.
}
