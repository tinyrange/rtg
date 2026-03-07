package main

type simpleErr struct{}

func (e simpleErr) Error() string { return "boom" }

func helper() error {
	return simpleErr{}
}

func main() {
	err := helper()
	if err != nil {
		_ = err.Error() // Selfhost unresolved call repro shape.
	}
}
