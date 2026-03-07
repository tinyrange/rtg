package main

type DriverOptions struct{ Strict bool }
type FrontendOptions struct{ Strict bool }

func frontendOptionsFromDriver(d DriverOptions) FrontendOptions {
	return FrontendOptions{Strict: d.Strict}
}

func main() {
	d := DriverOptions{Strict: true}
	_ = frontendOptionsFromDriver(d) // Deterministic drift repro shape in main option-conversion path.
}
