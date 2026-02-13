package main

import "fmt"

type Item struct {
	Name string
	Deps []string
}

type Registry struct {
	Items map[string]*Item
}

func (r *Registry) visit(name string, visited map[string]bool) {
	if visited[name] {
		return
	}
	visited[name] = true

	item := r.Items[name]
	if item == nil {
		fmt.Printf("NOT FOUND: looking for item\n")
		return
	}

	for _, dep := range item.Deps {
		r.visit(dep, visited)
	}
	fmt.Printf("visited: %d deps\n", len(item.Deps))
}

func main() {
	a := &Item{Name: "a", Deps: []string{"b"}}
	b := &Item{Name: "b", Deps: nil}

	reg := &Registry{Items: make(map[string]*Item)}
	reg.Items["a"] = a
	reg.Items["b"] = b

	visited := make(map[string]bool)
	reg.visit("a", visited)
	fmt.Printf("PASS\n")
}
