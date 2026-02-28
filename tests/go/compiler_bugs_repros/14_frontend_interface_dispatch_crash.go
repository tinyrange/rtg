package main

type Frontend interface {
	Compile() error
}

type defaultFrontend struct{}

func (f defaultFrontend) Compile() error { return nil }

func main() {
	var f Frontend = defaultFrontend{}
	_ = f.Compile() // Repro shape for interface-based frontend dispatch instability.
}
