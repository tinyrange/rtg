package main

func main() {
	type T struct{ X int }
	_ = T{X: 1}
}
