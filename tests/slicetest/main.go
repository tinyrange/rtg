package main

import (
	"fmt"
	"os"
)

type Item struct {
	name string
}

func (it Item) Name() string {
	return it.name
}

func makeItems() []Item {
	var items []Item
	items = append(items, Item{name: "alpha"})
	items = append(items, Item{name: "beta"})
	return items
}

func main() {
	items := makeItems()
	fmt.Printf("count: %d\n", len(items))
	for _, item := range items {
		n := item.Name()
		fmt.Printf("name: %s\n", n)
		if n == "" {
			fmt.Printf("ERROR: empty name\n")
			os.Exit(1)
		}
	}
	fmt.Printf("done\n")
}
