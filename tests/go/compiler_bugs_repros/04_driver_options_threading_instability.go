package main

type DriverOptions struct {
	Verbose bool
}

func helperWithOptions(opts DriverOptions, in string) string {
	if opts.Verbose {
		return in + "!"
	}
	return in
}

func helperWithoutOptions(in string) string {
	return in
}

func main() {
	opts := DriverOptions{}
	_, _ = helperWithOptions(opts, "x"), helperWithoutOptions("x") // Stage drift repro shape.
}
