package main

type T struct{}

func (T) M() {}

func main() {
	T{}.M() // Parser currently rejects unnamed receiver identifiers in selfhost mode.
}
