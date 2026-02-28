package main

type embedEntry struct {
	Name string
	Data string
}

var embedded = []embedEntry{
	{Name: "runtime/runtime_linux_amd64.go", Data: "// wasi runtime bytes ..."},
}

func main() {
	_ = embedded // Repro scaffold for embed name/data association corruption.
}
