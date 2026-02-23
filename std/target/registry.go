package target

import (
	"fmt"
	"sort"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// Defaults are compile-time target properties declared by a target package.
type Defaults struct {
	GOOS     string
	GOARCH   string
	PtrSize  int
	WordSize int
	Backend  string
}

// Driver carries runtime behavior for a target package.
type Driver interface {
	Configure(*common.Target) error
	Generate(*common.Target, *ir.IRModule, string) error
}

// Spec describes one target package registration.
type Spec struct {
	Triple      string
	PackagePath string
	Defaults    Defaults
	Driver      Driver
}

var registered = map[string]Spec{}

func Register(spec Spec) {
	if spec.Triple == "" {
		panic("target.Register: empty Triple")
	}
	if spec.PackagePath == "" {
		panic("target.Register: empty PackagePath for " + spec.Triple)
	}
	if _, exists := registered[spec.Triple]; exists {
		panic("target.Register: duplicate target registration for " + spec.Triple)
	}
	registered[spec.Triple] = spec
}

func Lookup(triple string) (Spec, bool) {
	spec, ok := registered[triple]
	return spec, ok
}

func Apply(triple string, tgt *common.Target) (Spec, bool, error) {
	spec, ok := Lookup(triple)
	if !ok {
		return Spec{}, false, nil
	}

	if spec.Defaults.Backend != "" {
		tgt.Backend = spec.Defaults.Backend
	} else {
		tgt.Backend = "native"
	}
	tgt.CModel = 0
	tgt.GOOS = spec.Defaults.GOOS
	tgt.GOARCH = spec.Defaults.GOARCH
	tgt.PtrSize = spec.Defaults.PtrSize
	tgt.WordSize = spec.Defaults.WordSize

	if spec.Driver != nil {
		err := spec.Driver.Configure(tgt)
		if err != nil {
			return Spec{}, true, fmt.Errorf("target %s (%s): %w", spec.Triple, spec.PackagePath, err)
		}
	}
	return spec, true, nil
}

func Generate(triple string, tgt *common.Target, irmod *ir.IRModule, outputPath string) (bool, error) {
	spec, ok := Lookup(triple)
	if !ok || spec.Driver == nil {
		return false, nil
	}
	return true, spec.Driver.Generate(tgt, irmod, outputPath)
}

func RegisteredTriples() []string {
	var triples []string
	for triple := range registered {
		triples = append(triples, triple)
	}
	sort.Strings(triples)
	return triples
}
