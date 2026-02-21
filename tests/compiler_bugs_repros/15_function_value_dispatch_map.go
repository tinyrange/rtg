package main

func genA() {}

func main() {
	generators := map[string]func(){
		"a": genA,
	}
	gen := generators["a"]
	gen() // Selfhost unresolved call repro shape: unresolved calls: gen.
}
