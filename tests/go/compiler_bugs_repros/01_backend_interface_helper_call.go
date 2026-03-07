package main

type Backend interface {
	Name() string
}

type nativeBackend struct{}

func (b nativeBackend) Name() string { return "native" }

func getRegisteredBackend(name string) (Backend, bool) {
	if name == "native" {
		return nativeBackend{}, true
	}
	return nil, false
}

func main() {
	b, ok := getRegisteredBackend("native")
	if !ok {
		return
	}
	_ = b.Name() // C selfhost repro shape: interface value returned from helper, then method call.
}
