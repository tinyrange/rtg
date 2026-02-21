package main

type Backend interface {
	Emit() string
}

type cBackend struct{}

func (b cBackend) Emit() string { return "c" }

func main() {
	registry := map[string]Backend{
		"c": cBackend{},
	}
	_ = registry["c"].Emit() // Interface-valued map lookup dispatch drift repro shape.
}
