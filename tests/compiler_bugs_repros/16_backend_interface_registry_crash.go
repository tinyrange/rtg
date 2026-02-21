package main

type Backend interface {
	Compile() error
}

type nativeBackend struct{}

func (b nativeBackend) Compile() error { return nil }

func main() {
	registry := map[string]Backend{"native": nativeBackend{}}
	_ = registry["native"].Compile() // Repro shape for backend interface dispatch instability.
}
