package target

import (
	"fmt"
	"sort"
	"strings"

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

// ABIProvider is a typed, map-backed ABI payload used by target packages and
// external single-file target definitions.
type ABIProvider struct {
	Kind string
	U64  map[string]uint64
	I64  map[string]int64
	Str  map[string]string
	Bool map[string]bool
}

// GenericABI is an alias used by target definition sources.
type GenericABI = ABIProvider

func normalizeABI(abi ABIProvider) ABIProvider {
	if abi.Kind == "" {
		abi.Kind = "none"
	}
	return abi
}

func ABIKind(abi ABIProvider) string {
	if abi.Kind == "" {
		return "none"
	}
	return abi.Kind
}

func ABIUint64(abi ABIProvider, name string, fallback uint64) uint64 {
	if abi.U64 == nil {
		return fallback
	}
	v, ok := abi.U64[name]
	if !ok {
		return fallback
	}
	return v
}

func ABIInt64(abi ABIProvider, name string, fallback int64) int64 {
	if abi.I64 == nil {
		return fallback
	}
	v, ok := abi.I64[name]
	if !ok {
		return fallback
	}
	return v
}

func ABIString(abi ABIProvider, name string, fallback string) string {
	if abi.Str == nil {
		return fallback
	}
	v, ok := abi.Str[name]
	if !ok {
		return fallback
	}
	return v
}

func ABIBool(abi ABIProvider, name string, fallback bool) bool {
	if abi.Bool == nil {
		return fallback
	}
	v, ok := abi.Bool[name]
	if !ok {
		return fallback
	}
	return v
}

// Spec describes one target package registration.
type Spec struct {
	Triple      string
	PackagePath string
	Defaults    Defaults
	Driver      Driver
	Assembler   string
	BinFormat   string
}

var registered = map[string]Spec{}
var registeredABI = map[string]ABIProvider{}
var registeredAssemblers = map[string]string{}
var registeredBinFormats = map[string]string{}

func init() {
	RegisterAssembler("aarch64", "builtin.aarch64")
	RegisterBinFormat("macho64", "builtin.macho64")
}

func Register(spec Spec) {
	if spec.Triple == "" {
		panic("target.Register: empty Triple")
	}
	if spec.PackagePath == "" {
		panic("target.Register: empty PackagePath for " + spec.Triple)
	}
	if prev, exists := registered[spec.Triple]; exists {
		if prev.PackagePath == spec.PackagePath {
			return
		}
		panic("target.Register: duplicate target registration for " + spec.Triple)
	}
	registered[spec.Triple] = spec
}

// RegisterExternal registers or replaces a target spec loaded from -target-file
// or -target-root input at compiler startup.
func RegisterExternal(spec Spec) {
	if spec.Triple == "" {
		panic("target.RegisterExternal: empty Triple")
	}
	if spec.PackagePath == "" {
		panic("target.RegisterExternal: empty PackagePath for " + spec.Triple)
	}
	registered[spec.Triple] = spec
}

func parseEncodedABI(encoded string) ABIProvider {
	abi := ABIProvider{
		Kind: "none",
		U64:  make(map[string]uint64),
		I64:  make(map[string]int64),
		Str:  make(map[string]string),
		Bool: make(map[string]bool),
	}
	if encoded == "" {
		return abi
	}
	lines := strings.Split(encoded, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := line[0:eq]
		val := line[eq+1 : len(line)]
		if key == "kind" {
			abi.Kind = val
			continue
		}
		if len(key) <= 2 || key[1] != ':' {
			continue
		}
		name := key[2:len(key)]
		switch key[0] {
		case 'u':
			n, ok := parseIntLiteral(val)
			if ok && n >= 0 {
				abi.U64[name] = uint64(n)
			}
		case 'i':
			n, ok := parseIntLiteral(val)
			if ok {
				abi.I64[name] = n
			}
		case 's':
			abi.Str[name] = val
		case 'b':
			abi.Bool[name] = val == "1" || val == "true"
		}
	}
	return normalizeABI(abi)
}

func parseIntLiteral(raw string) (int64, bool) {
	if raw == "" {
		return 0, false
	}
	sign := int64(1)
	i := 0
	if raw[0] == '-' {
		sign = -1
		i = 1
	}
	base := int64(10)
	if i+2 <= len(raw) && raw[i] == '0' && i+1 < len(raw) {
		if raw[i+1] == 'x' || raw[i+1] == 'X' {
			base = 16
			i = i + 2
		} else if raw[i+1] == 'b' || raw[i+1] == 'B' {
			base = 2
			i = i + 2
		} else if raw[i+1] == 'o' || raw[i+1] == 'O' {
			base = 8
			i = i + 2
		}
	}
	var v int64
	for i < len(raw) {
		ch := raw[i]
		i = i + 1
		if ch == '_' {
			continue
		}
		d := int64(-1)
		if ch >= '0' && ch <= '9' {
			d = int64(ch - '0')
		} else if ch >= 'a' && ch <= 'f' {
			d = int64(ch-'a') + 10
		} else if ch >= 'A' && ch <= 'F' {
			d = int64(ch-'A') + 10
		}
		if d < 0 || d >= base {
			return 0, false
		}
		v = v*base + d
	}
	return sign * v, true
}

func RegisterABI(triple string, encoded string) {
	if triple == "" {
		panic("target.RegisterABI: empty Triple")
	}
	if _, exists := registeredABI[triple]; exists {
		return
	}
	registeredABI[triple] = parseEncodedABI(encoded)
}

func RegisterExternalABI(triple string, abi ABIProvider) {
	if triple == "" {
		panic("target.RegisterExternalABI: empty Triple")
	}
	registeredABI[triple] = normalizeABI(abi)
}

func LookupABI(triple string) (ABIProvider, bool) {
	abi, ok := registeredABI[triple]
	return abi, ok
}

func RegisterAssembler(name string, provider string) {
	if name == "" {
		panic("target.RegisterAssembler: empty name")
	}
	if _, exists := registeredAssemblers[name]; exists {
		return
	}
	registeredAssemblers[name] = provider
}

func LookupAssembler(name string) (string, bool) {
	provider, ok := registeredAssemblers[name]
	return provider, ok
}

func RegisterBinFormat(name string, provider string) {
	if name == "" {
		panic("target.RegisterBinFormat: empty name")
	}
	if _, exists := registeredBinFormats[name]; exists {
		return
	}
	registeredBinFormats[name] = provider
}

func LookupBinFormat(name string) (string, bool) {
	provider, ok := registeredBinFormats[name]
	return provider, ok
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
		driver := spec.Driver
		err := driver.Configure(tgt)
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
	driver := spec.Driver
	return true, driver.Generate(tgt, irmod, outputPath)
}

func RegisteredTriples() []string {
	var triples []string
	for triple := range registered {
		triples = append(triples, triple)
	}
	sort.Strings(triples)
	return triples
}
