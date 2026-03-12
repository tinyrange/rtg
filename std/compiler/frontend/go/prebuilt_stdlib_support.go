package frontend

import (
	"fmt"
	"os"
	"strings"

	"j5.nz/rtg/std/compiler/binary"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
	"j5.nz/rtg/std/compiler/stdlib"
)

func targetHasBuildTag(target *common.Target, tag string) bool {
	if target == nil {
		return false
	}
	for _, existing := range target.BuildTags {
		if existing == tag {
			return true
		}
	}
	return false
}

func shouldUsePrebuiltStdlib(target *common.Target) bool {
	if target == nil {
		return false
	}
	if target.GOOS != "linux" || target.GOARCH != "amd64" {
		return false
	}
	if targetHasBuildTag(target, "regen_prebuilt_stdlib") {
		return false
	}
	return targetHasBuildTag(target, "rtg") && targetHasBuildTag(target, "no_embed_std")
}

func loadPrebuiltStdlibPackage(baseDir string, candidate string, importPath string) *Package {
	data := loadPrebuiltStdlibFile(baseDir, "std/compiler/frontend/go/prebuilt_stdlib_summaries/"+candidate+".ps", "compiler/frontend/go/prebuilt_stdlib_summaries/"+candidate+".ps")
	if data == "" {
		return nil
	}
	name, files, err := decodePrebuiltPackageSummary(data)
	if err != nil {
		panic(fmt.Sprintf("invalid prebuilt summary for %s: %v", candidate, err))
	}
	pkg := &Package{
		Name:     name,
		Path:     importPath,
		Dir:      candidate,
		Files:    files,
		Symbols:  make(map[string]*Symbol),
		Prebuilt: true,
	}
	pkg.Imports = collectImports(pkg)
	collectSymbols(pkg)
	return pkg
}

func loadPrebuiltStdlibIR(baseDir string) (*ir.IRModule, error) {
	data := loadPrebuiltStdlibFile(baseDir, "std/compiler/frontend/go/prebuilt_stdlib.irt", "compiler/frontend/go/prebuilt_stdlib.irt")
	if data == "" {
		return nil, nil
	}
	return binary.ReadIRTextData("embedded prebuilt stdlib IR", data)
}

func loadPrebuiltStdlibFile(baseDir string, diskPath string, embedPath string) string {
	if baseDir != "" {
		path := common.TrimTrailingSlash(common.NormalizePath(baseDir)) + "/" + common.NormalizePath(diskPath)
		data, err := os.ReadFile(path)
		if err == nil {
			return string(data)
		}
	}
	return stdlib.ReadFileFromEmbed(embedPath)
}

type prebuiltSummaryNode struct {
	kind     NodeKind
	name     string
	x        int
	y        int
	body     int
	typ      int
	children []int
}

func decodePrebuiltPackageSummary(data string) (string, []*Node, error) {
	lines := strings.Split(data, "\n")
	nodes := make(map[int]prebuiltSummaryNode)
	var fileIDs []int
	name := ""
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "pkg":
			if len(fields) != 2 {
				return "", nil, fmt.Errorf("invalid pkg record")
			}
			decoded, err := decodePrebuiltHex(fields[1])
			if err != nil {
				return "", nil, err
			}
			name = decoded
		case "files":
			if len(fields) != 2 {
				return "", nil, fmt.Errorf("invalid files record")
			}
			ids, err := decodePrebuiltIntList(fields[1])
			if err != nil {
				return "", nil, err
			}
			fileIDs = ids
		case "n":
			if len(fields) != 9 {
				return "", nil, fmt.Errorf("invalid node record")
			}
			id, err := decodePrebuiltInt(fields[1])
			if err != nil {
				return "", nil, err
			}
			kind, err := decodePrebuiltInt(fields[2])
			if err != nil {
				return "", nil, err
			}
			nodeName, err := decodePrebuiltHex(fields[3])
			if err != nil {
				return "", nil, err
			}
			x, err := decodePrebuiltInt(fields[4])
			if err != nil {
				return "", nil, err
			}
			y, err := decodePrebuiltInt(fields[5])
			if err != nil {
				return "", nil, err
			}
			body, err := decodePrebuiltInt(fields[6])
			if err != nil {
				return "", nil, err
			}
			typ, err := decodePrebuiltInt(fields[7])
			if err != nil {
				return "", nil, err
			}
			children, err := decodePrebuiltIntList(fields[8])
			if err != nil {
				return "", nil, err
			}
			nodes[id] = prebuiltSummaryNode{
				kind:     NodeKind(kind),
				name:     nodeName,
				x:        x,
				y:        y,
				body:     body,
				typ:      typ,
				children: children,
			}
		default:
			return "", nil, fmt.Errorf("unknown record %q", fields[0])
		}
	}
	if name == "" {
		return "", nil, fmt.Errorf("missing package name")
	}

	built := make(map[int]*Node, len(nodes))
	for id, record := range nodes {
		built[id] = &Node{Kind: record.kind, Name: record.name}
	}
	for id, record := range nodes {
		node := built[id]
		node.X = built[record.x]
		node.Y = built[record.y]
		node.Body = built[record.body]
		node.Type = built[record.typ]
		if len(record.children) > 0 {
			node.Nodes = make([]*Node, 0, len(record.children))
			for _, childID := range record.children {
				node.Nodes = append(node.Nodes, built[childID])
			}
		}
	}

	files := make([]*Node, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		fileNode := built[fileID]
		if fileNode == nil {
			return "", nil, fmt.Errorf("missing file node %d", fileID)
		}
		files = append(files, fileNode)
	}
	return name, files, nil
}

func decodePrebuiltInt(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	sign := 1
	if s[0] == '-' {
		sign = -1
		s = s[1:]
	}
	v := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid integer %q", s)
		}
		v = v*10 + int(ch-'0')
	}
	return sign * v, nil
}

func decodePrebuiltIntList(s string) ([]int, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		v, err := decodePrebuiltInt(part)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func decodePrebuiltHex(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if len(s)%2 != 0 {
		return "", fmt.Errorf("invalid hex length")
	}
	buf := make([]byte, len(s)/2)
	for i := 0; i < len(buf); i++ {
		hi, ok := decodePrebuiltNibble(s[i*2])
		if !ok {
			return "", fmt.Errorf("invalid hex")
		}
		lo, ok := decodePrebuiltNibble(s[i*2+1])
		if !ok {
			return "", fmt.Errorf("invalid hex")
		}
		buf[i] = byte(hi<<4 | lo)
	}
	return string(buf), nil
}

func decodePrebuiltNibble(ch byte) (int, bool) {
	switch {
	case ch >= '0' && ch <= '9':
		return int(ch - '0'), true
	case ch >= 'a' && ch <= 'f':
		return int(ch-'a') + 10, true
	case ch >= 'A' && ch <= 'F':
		return int(ch-'A') + 10, true
	}
	return 0, false
}

func moduleHasPrebuiltStdlib(mod *Module) bool {
	if mod == nil {
		return false
	}
	for _, path := range mod.Order {
		pkg, ok := mod.Packages[path]
		if ok && pkg != nil && pkg.Prebuilt {
			return true
		}
	}
	return false
}

func (c *Compiler) seedPrebuiltStdlibState(irmod *ir.IRModule) error {
	if irmod == nil {
		return nil
	}
	for typeName, id := range irmod.TypeIDs {
		c.typeIDs[typeName] = id
		if id >= c.nextTypeID {
			c.nextTypeID = id + 1
		}
	}
	for key, value := range irmod.MethodTable {
		c.methodTable[key] = value
	}
	for key, value := range irmod.IfaceMethods {
		c.ifaceMethods[key] = value
	}
	for key, value := range irmod.IfaceMethodRets {
		c.ifaceMethodRets[key] = value
	}
	return nil
}

func (c *Compiler) mergePrebuiltStdlibIR(irmod *ir.IRModule) error {
	if irmod == nil {
		return nil
	}
	if len(irmod.Globals) > len(c.irmod.Globals) {
		return fmt.Errorf("prebuilt stdlib global table larger than compiler global table")
	}
	for i := 0; i < len(irmod.Globals); i++ {
		if c.irmod.Globals[i].Name != irmod.Globals[i].Name {
			return fmt.Errorf("prebuilt stdlib global order mismatch at %d", i)
		}
		if c.irmod.Globals[i].Type == nil && irmod.Globals[i].Type != nil {
			c.irmod.Globals[i].Type = irmod.Globals[i].Type
		}
	}
	c.irmod.Funcs = append(c.irmod.Funcs, irmod.Funcs...)
	if len(irmod.Types) > 0 {
		c.irmod.Types = append(c.irmod.Types, irmod.Types...)
	}
	if len(irmod.LinkStaticFuncs) > 0 {
		if c.irmod.LinkStaticFuncs == nil {
			c.irmod.LinkStaticFuncs = make(map[string]string)
		}
		for name, spec := range irmod.LinkStaticFuncs {
			c.irmod.LinkStaticFuncs[name] = spec
		}
	}
	if len(irmod.CallbackFuncs) > 0 {
		if c.irmod.CallbackFuncs == nil {
			c.irmod.CallbackFuncs = make(map[string]bool)
		}
		for name, enabled := range irmod.CallbackFuncs {
			if enabled {
				c.irmod.CallbackFuncs[name] = true
			}
		}
	}
	return nil
}
