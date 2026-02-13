package main

import (
	"fmt"
	"strings"
)

type Target struct {
	Name         string
	Dependencies []string
}

type Buildfile struct {
	Targets map[string]*Target
}

func main() {
	bf := &Buildfile{
		Targets: make(map[string]*Target),
	}
	bf.Targets["default"] = &Target{
		Name:         "default",
		Dependencies: make([]string, 0),
	}
	bf.Targets["default"].Dependencies = append(bf.Targets["default"].Dependencies, "test")

	bf.Targets["test"] = &Target{
		Name: "test",
	}

	var names []string
	for name := range bf.Targets {
		names = append(names, name)
		fmt.Printf("found target: %s\n", name)
	}

	fmt.Printf("count: %d\n", len(names))

	for _, name := range names {
		fmt.Printf("listing: %s\n", name)
		target := bf.Targets[name]
		fmt.Printf("  target name: %s\n", target.Name)
		fmt.Printf("  deps len: %d\n", len(target.Dependencies))
		if len(target.Dependencies) > 0 {
			deps := strings.Join(target.Dependencies, ", ")
			fmt.Printf("  deps: %s\n", deps)
		}
	}

	fmt.Printf("done\n")
}
