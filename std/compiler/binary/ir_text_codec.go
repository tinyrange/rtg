package binary

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"j5.nz/rtg/std/compiler/ir"
)

const (
	irTextMagicLine = "rtgir 2"
)

var typeKindNames = []string{
	"VOID",
	"BOOL",
	"BYTE",
	"INT32",
	"INT",
	"UINTPTR",
	"STRING",
	"POINTER",
	"SLICE",
	"STRUCT",
	"INTERFACE",
	"FUNC",
	"MAP",
}

var typeKindByName = map[string]ir.TypeKind{}

var opcodeNames = []string{
	"CONST_I64",
	"CONST_STR",
	"CONST_BOOL",
	"CONST_NIL",
	"LOCAL_GET",
	"LOCAL_SET",
	"LOCAL_ADD_IMM",
	"LOCAL_ADDR",
	"GLOBAL_GET",
	"GLOBAL_SET",
	"GLOBAL_ADDR",
	"DROP",
	"DUP",
	"ADD",
	"SUB",
	"MUL",
	"DIV",
	"MOD",
	"NEG",
	"AND",
	"OR",
	"XOR",
	"SHL",
	"SHR",
	"EQ",
	"NEQ",
	"LT",
	"GT",
	"LEQ",
	"GEQ",
	"JMP_EQ",
	"JMP_NEQ",
	"JMP_LT",
	"JMP_GT",
	"JMP_LEQ",
	"JMP_GEQ",
	"NOT",
	"LOAD",
	"STORE",
	"OFFSET",
	"LABEL",
	"JMP",
	"JMP_IF",
	"JMP_IF_NOT",
	"CALL",
	"CALL_INTRINSIC",
	"RETURN",
	"SLICE_GET",
	"SLICE_MAKE",
	"STRING_GET",
	"STRING_MAKE",
	"INDEX_ADDR",
	"LEN",
	"CONVERT",
	"IFACE_BOX",
	"IFACE_CALL",
	"PANIC",
	"CAP",
}

var opcodeByName = map[string]ir.Opcode{}

func init() {
	for i := 0; i < len(typeKindNames); i++ {
		typeKindByName[typeKindNames[i]] = ir.TypeKind(i)
	}
	for i := 0; i < len(opcodeNames); i++ {
		opcodeByName[opcodeNames[i]] = ir.Opcode(i)
	}
}

func OpcodeName(op ir.Opcode) string {
	if int(op) < 0 || int(op) >= len(opcodeNames) {
		return "OP_" + strconv.Itoa(int(op))
	}
	return opcodeNames[int(op)]
}

func opRequiresArg(op ir.Opcode) bool {
	switch op {
	case ir.OP_CONST_BOOL,
		ir.OP_LOCAL_GET, ir.OP_LOCAL_SET, ir.OP_LOCAL_ADD_IMM, ir.OP_LOCAL_ADDR,
		ir.OP_GLOBAL_GET, ir.OP_GLOBAL_SET, ir.OP_GLOBAL_ADDR,
		ir.OP_LOAD, ir.OP_STORE, ir.OP_OFFSET,
		ir.OP_LABEL, ir.OP_JMP, ir.OP_JMP_IF, ir.OP_JMP_IF_NOT,
		ir.OP_JMP_EQ, ir.OP_JMP_NEQ, ir.OP_JMP_LT, ir.OP_JMP_GT, ir.OP_JMP_LEQ, ir.OP_JMP_GEQ,
		ir.OP_CALL, ir.OP_CALL_INTRINSIC, ir.OP_RETURN,
		ir.OP_SLICE_GET, ir.OP_SLICE_MAKE, ir.OP_STRING_GET, ir.OP_STRING_MAKE, ir.OP_INDEX_ADDR,
		ir.OP_IFACE_BOX, ir.OP_IFACE_CALL:
		return true
	}
	return false
}

func opRequiresVal(op ir.Opcode) bool {
	switch op {
	case ir.OP_CONST_I64, ir.OP_LOCAL_ADD_IMM:
		return true
	}
	return false
}

func opRequiresName(op ir.Opcode) bool {
	switch op {
	case ir.OP_CONST_STR, ir.OP_CALL, ir.OP_CALL_INTRINSIC, ir.OP_CONVERT, ir.OP_IFACE_CALL:
		return true
	}
	return false
}

type textTypeRec struct {
	kind    ir.TypeKind
	name    string
	pkg     string
	size    int
	align   int
	elemIdx int
	keyIdx  int
	fields  []textFieldRec
	params  []int
	results []int
}

type textFieldRec struct {
	name    string
	typeIdx int
	offset  int
}

type textGlobalRec struct {
	name    string
	typeIdx int
	index   int
}

type textLocalRec struct {
	local   ir.IRLocal
	typeIdx int
}

type textFuncRec struct {
	name   string
	params int
	rets   int
	locals []textLocalRec
	code   []ir.Inst
	native *ir.NativeFunc
}

func textCollectTypeInfo(t *ir.TypeInfo, idx map[*ir.TypeInfo]int, all *[]*ir.TypeInfo) {
	if t == nil {
		return
	}
	if _, ok := idx[t]; ok {
		return
	}
	idx[t] = len(*all)
	*all = append(*all, t)
	textCollectTypeInfo(t.Elem, idx, all)
	textCollectTypeInfo(t.Key, idx, all)
	for _, f := range t.Fields {
		textCollectTypeInfo(f.Type, idx, all)
	}
	for _, p := range t.Params {
		textCollectTypeInfo(p, idx, all)
	}
	for _, r := range t.Results {
		textCollectTypeInfo(r, idx, all)
	}
}

func textTypeIndex(idx map[*ir.TypeInfo]int, t *ir.TypeInfo) int {
	if t == nil {
		return -1
	}
	return idx[t]
}

func textQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			if ch < 32 {
				b.WriteString("\\x")
				b.WriteByte(textHexChar((ch >> 4) & 0x0f))
				b.WriteByte(textHexChar(ch & 0x0f))
			} else {
				b.WriteByte(ch)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func sortedMapKeysStringString(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedMapKeysStringInt(m map[string]int) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedMapKeysSet(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type textBufferedWriter struct {
	f   *os.File
	buf []byte
	err error
}

func newTextBufferedWriter(f *os.File) *textBufferedWriter {
	return &textBufferedWriter{
		f:   f,
		buf: make([]byte, 0, 4096),
	}
}

func (w *textBufferedWriter) WriteByte(ch byte) {
	if w.err != nil {
		return
	}
	if len(w.buf) == cap(w.buf) {
		w.flush()
		if w.err != nil {
			return
		}
	}
	w.buf = append(w.buf, ch)
}

func (w *textBufferedWriter) WriteString(s string) {
	if w.err != nil {
		return
	}
	for len(s) > 0 {
		if len(w.buf) == cap(w.buf) {
			w.flush()
			if w.err != nil {
				return
			}
		}
		space := cap(w.buf) - len(w.buf)
		if space > len(s) {
			space = len(s)
		}
		w.buf = append(w.buf, s[:space]...)
		s = s[space:]
	}
}

func (w *textBufferedWriter) flush() {
	if w.err != nil || len(w.buf) == 0 {
		return
	}
	start := 0
	for start < len(w.buf) {
		n, err := w.f.Write(w.buf[start:])
		if n > 0 {
			start += n
		}
		if err != nil {
			if start >= len(w.buf) {
				break
			}
			w.err = err
			return
		}
		if n == 0 {
			w.err = fmt.Errorf("write made no progress")
			return
		}
	}
	w.buf = w.buf[:0]
}

func (w *textBufferedWriter) Finish() error {
	w.flush()
	return w.err
}

func writeInt(w *textBufferedWriter, v int) {
	w.WriteString(strconv.Itoa(v))
}

func writeInt64(w *textBufferedWriter, v int64) {
	writeInt64Hex(w, v)
}

func writeInt64Hex(w *textBufferedWriter, v int64) {
	u := uint64(v)
	w.WriteString("0x")
	started := false
	shift := 60
	for {
		nib := byte((u >> uint(shift)) & 0x0f)
		if started || nib != 0 || shift == 0 {
			started = true
			w.WriteByte(textHexChar(nib))
		}
		if shift == 0 {
			break
		}
		shift = shift - 4
	}
}

func WriteIRText(irmod *ir.IRModule, path string) error {
	if irmod == nil {
		return fmt.Errorf("nil IR module")
	}

	typeIdx := make(map[*ir.TypeInfo]int)
	var allTypes []*ir.TypeInfo
	for _, t := range irmod.Types {
		textCollectTypeInfo(t, typeIdx, &allTypes)
	}
	for i := 0; i < len(irmod.Globals); i++ {
		textCollectTypeInfo(irmod.Globals[i].Type, typeIdx, &allTypes)
	}
	for _, f := range irmod.Funcs {
		for i := 0; i < len(f.Locals); i++ {
			textCollectTypeInfo(f.Locals[i].Type, typeIdx, &allTypes)
		}
	}

	var outFile *os.File
	closeOutput := false
	if path == "-" {
		outFile = os.Stdout
	} else {
		var err error
		outFile, err = os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		closeOutput = true
	}
	w := newTextBufferedWriter(outFile)

	w.WriteString(irTextMagicLine)
	w.WriteString("\n\n")

	w.WriteString("types {\n")
	for i := 0; i < len(allTypes); i++ {
		t := allTypes[i]
		kindName := "UNKNOWN"
		if int(t.Kind) >= 0 && int(t.Kind) < len(typeKindNames) {
			kindName = typeKindNames[int(t.Kind)]
		}
		w.WriteString("  type @")
		writeInt(w, i)
		w.WriteString(" kind=")
		w.WriteString(kindName)
		w.WriteString(" name=")
		w.WriteString(textQuote(t.Name))
		w.WriteString(" pkg=")
		w.WriteString(textQuote(t.Pkg))
		w.WriteString(" size=")
		writeInt(w, t.Size)
		w.WriteString(" align=")
		writeInt(w, t.Align)
		w.WriteString(" elem=@")
		writeInt(w, textTypeIndex(typeIdx, t.Elem))
		w.WriteString(" key=@")
		writeInt(w, textTypeIndex(typeIdx, t.Key))
		w.WriteString("\n")
	}
	for owner := 0; owner < len(allTypes); owner++ {
		t := allTypes[owner]
		for i := 0; i < len(t.Fields); i++ {
			f := t.Fields[i]
			w.WriteString("  field owner=@")
			writeInt(w, owner)
			w.WriteString(" name=")
			w.WriteString(textQuote(f.Name))
			w.WriteString(" type=@")
			writeInt(w, textTypeIndex(typeIdx, f.Type))
			w.WriteString(" offset=")
			writeInt(w, f.Offset)
			w.WriteString("\n")
		}
		for i := 0; i < len(t.Params); i++ {
			w.WriteString("  param owner=@")
			writeInt(w, owner)
			w.WriteString(" type=@")
			writeInt(w, textTypeIndex(typeIdx, t.Params[i]))
			w.WriteString("\n")
		}
		for i := 0; i < len(t.Results); i++ {
			w.WriteString("  result owner=@")
			writeInt(w, owner)
			w.WriteString(" type=@")
			writeInt(w, textTypeIndex(typeIdx, t.Results[i]))
			w.WriteString("\n")
		}
	}
	w.WriteString("}\n\n")

	w.WriteString("root_types {\n")
	for i := 0; i < len(irmod.Types); i++ {
		w.WriteString("  root type=@")
		writeInt(w, textTypeIndex(typeIdx, irmod.Types[i]))
		w.WriteString("\n")
	}
	w.WriteString("}\n\n")

	w.WriteString("globals {\n")
	for i := 0; i < len(irmod.Globals); i++ {
		g := irmod.Globals[i]
		w.WriteString("  global name=")
		w.WriteString(textQuote(g.Name))
		w.WriteString(" type=@")
		writeInt(w, textTypeIndex(typeIdx, g.Type))
		w.WriteString(" index=")
		writeInt(w, g.Index)
		w.WriteString("\n")
	}
	w.WriteString("}\n\n")

	w.WriteString("funcs {\n")
	for i := 0; i < len(irmod.Funcs); i++ {
		f := irmod.Funcs[i]
		w.WriteString("  func ")
		w.WriteString(textQuote(f.Name))
		w.WriteString(" params=")
		writeInt(w, f.Params)
		w.WriteString(" rets=")
		writeInt(w, f.RetCount)
		w.WriteString(" {\n")

		w.WriteString("    locals {\n")
		for j := 0; j < len(f.Locals); j++ {
			l := f.Locals[j]
			w.WriteString("      local name=")
			w.WriteString(textQuote(l.Name))
			w.WriteString(" type=@")
			writeInt(w, textTypeIndex(typeIdx, l.Type))
			w.WriteString(" index=")
			writeInt(w, l.Index)
			w.WriteString(" is64=")
			if l.Is64 {
				w.WriteString("true")
			} else {
				w.WriteString("false")
			}
			w.WriteString(" width=")
			writeInt(w, l.Width)
			w.WriteString("\n")
		}
		w.WriteString("    }\n")

		w.WriteString("    code {\n")
		for j := 0; j < len(f.Code); j++ {
			in := f.Code[j]
			w.WriteString("      ")
			w.WriteString(OpcodeName(in.Op))
			if opRequiresArg(in.Op) || in.Arg != 0 {
				w.WriteString(" arg=")
				writeInt(w, in.Arg)
			}
			if opRequiresVal(in.Op) || in.Val != 0 {
				w.WriteString(" val=")
				writeInt64(w, in.Val)
			}
			if in.Width != 0 {
				w.WriteString(" width=")
				writeInt(w, in.Width)
			}
			if opRequiresName(in.Op) || in.Name != "" {
				w.WriteString(" name=")
				w.WriteString(textQuote(in.Name))
			}
			w.WriteString("\n")
		}
		w.WriteString("    }\n")

		if f.Native != nil {
			w.WriteString("    native {\n")
			w.WriteString("      arch value=")
			w.WriteString(textQuote(f.Native.Arch))
			w.WriteString("\n")
			w.WriteString("      bytes hex=")
			w.WriteString(textQuote(textHexEncode(f.Native.Code)))
			w.WriteString("\n")
			for j := 0; j < len(f.Native.Fixups); j++ {
				fix := f.Native.Fixups[j]
				w.WriteString("      fixup kind=")
				writeInt(w, fix.Kind)
				w.WriteString(" off=")
				writeInt(w, fix.Off)
				w.WriteString(" target=")
				w.WriteString(textQuote(fix.Target))
				w.WriteString("\n")
			}
			w.WriteString("    }\n")
		}

		w.WriteString("  }\n")
	}
	w.WriteString("}\n\n")

	w.WriteString("link_static_funcs {\n")
	for _, k := range sortedMapKeysStringString(irmod.LinkStaticFuncs) {
		w.WriteString("  entry key=")
		w.WriteString(textQuote(k))
		w.WriteString(" value=")
		w.WriteString(textQuote(irmod.LinkStaticFuncs[k]))
		w.WriteString("\n")
	}
	w.WriteString("}\n\n")

	w.WriteString("zero_call_funcs {\n")
	for _, name := range sortedMapKeysSet(irmod.ZeroCallFuncs) {
		w.WriteString("  entry name=")
		w.WriteString(textQuote(name))
		w.WriteString("\n")
	}
	w.WriteString("}\n\n")

	w.WriteString("type_ids {\n")
	for _, k := range sortedMapKeysStringInt(irmod.TypeIDs) {
		w.WriteString("  entry key=")
		w.WriteString(textQuote(k))
		w.WriteString(" value=")
		writeInt(w, irmod.TypeIDs[k])
		w.WriteString("\n")
	}
	w.WriteString("}\n\n")

	w.WriteString("method_table {\n")
	for _, k := range sortedMapKeysStringString(irmod.MethodTable) {
		w.WriteString("  entry key=")
		w.WriteString(textQuote(k))
		w.WriteString(" value=")
		w.WriteString(textQuote(irmod.MethodTable[k]))
		w.WriteString("\n")
	}
	w.WriteString("}\n\n")

	w.WriteString("iface_methods {\n")
	if len(irmod.IfaceMethods) > 0 {
		keys := make([]string, 0, len(irmod.IfaceMethods))
		for k := range irmod.IfaceMethods {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, iface := range keys {
			methods := irmod.IfaceMethods[iface]
			for i := 0; i < len(methods); i++ {
				w.WriteString("  entry iface=")
				w.WriteString(textQuote(iface))
				w.WriteString(" method=")
				w.WriteString(textQuote(methods[i]))
				w.WriteString("\n")
			}
		}
	}
	w.WriteString("}\n\n")

	w.WriteString("iface_method_rets {\n")
	for _, k := range sortedMapKeysStringInt(irmod.IfaceMethodRets) {
		w.WriteString("  entry key=")
		w.WriteString(textQuote(k))
		w.WriteString(" value=")
		writeInt(w, irmod.IfaceMethodRets[k])
		w.WriteString("\n")
	}
	w.WriteString("}\n\n")

	w.WriteString("callback_funcs {\n")
	for _, name := range sortedMapKeysSet(irmod.CallbackFuncs) {
		w.WriteString("  entry name=")
		w.WriteString(textQuote(name))
		w.WriteString("\n")
	}
	w.WriteString("}\n")

	if err := w.Finish(); err != nil {
		if closeOutput {
			_ = outFile.Close()
		}
		return err
	}
	if closeOutput {
		if err := outFile.Close(); err != nil {
			return err
		}
	}
	return nil
}

func textHexEncode(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	const hex = "0123456789abcdef"
	out := make([]byte, len(data)*2)
	for i := 0; i < len(data); i++ {
		v := data[i]
		out[i*2] = hex[(v>>4)&0x0f]
		out[i*2+1] = hex[v&0x0f]
	}
	return string(out)
}

func textHexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("hex string has odd length")
	}
	if s == "" {
		return nil, nil
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		hi, ok := textHexNibble(s[i*2])
		if !ok {
			return nil, fmt.Errorf("invalid hex char %q", s[i*2])
		}
		lo, ok := textHexNibble(s[i*2+1])
		if !ok {
			return nil, fmt.Errorf("invalid hex char %q", s[i*2+1])
		}
		out[i] = (hi << 4) | lo
	}
	return out, nil
}

func textHexNibble(ch byte) (byte, bool) {
	if ch >= '0' && ch <= '9' {
		return ch - '0', true
	}
	if ch >= 'a' && ch <= 'f' {
		return ch - 'a' + 10, true
	}
	if ch >= 'A' && ch <= 'F' {
		return ch - 'A' + 10, true
	}
	return 0, false
}

type textDecoder struct {
	seenHeader bool
	stack      []string
	curFunc    *textFuncRec
	attrsTmp   map[string]string

	types          []*textTypeRec
	rootTypeIdx    []int
	globals        []textGlobalRec
	funcs          []*textFuncRec
	linkStatic     map[string]string
	zeroCallFuncs  map[string]bool
	typeIDs        map[string]int
	methodTable    map[string]string
	ifaceMethods   map[string][]string
	ifaceMethodRet map[string]int
	callbackFuncs  map[string]bool
}

func (d *textDecoder) top() string {
	if len(d.stack) == 0 {
		return ""
	}
	return d.stack[len(d.stack)-1]
}

func (d *textDecoder) push(name string) {
	d.stack = append(d.stack, name)
}

func (d *textDecoder) pop() (string, bool) {
	if len(d.stack) == 0 {
		return "", false
	}
	out := d.stack[len(d.stack)-1]
	d.stack = d.stack[:len(d.stack)-1]
	return out, true
}

func (d *textDecoder) ensureTypeRecord(id int) error {
	if id < 0 {
		return fmt.Errorf("type id must be non-negative")
	}
	for len(d.types) <= id {
		d.types = append(d.types, nil)
	}
	if d.types[id] != nil {
		return fmt.Errorf("duplicate type id @%d", id)
	}
	d.types[id] = &textTypeRec{}
	return nil
}

func tokenizeTextLineInto(line string, out []string) ([]string, error) {
	out = out[:0]
	i := 0
	for i < len(line) {
		ch := line[i]
		if ch == ' ' || ch == '\t' || ch == '\r' {
			i++
			continue
		}
		if ch == '#' {
			break
		}
		if ch == '{' {
			out = append(out, "{")
			i++
			continue
		}
		if ch == '}' {
			out = append(out, "}")
			i++
			continue
		}
		if ch == '=' {
			out = append(out, "=")
			i++
			continue
		}
		if ch == '"' {
			rawStart := i + 1
			j := i + 1
			var decoded []byte
			sawEscape := false
			for j < len(line) {
				c := line[j]
				if c == '"' {
					j++
					break
				}
				if c != '\\' {
					if sawEscape {
						decoded = append(decoded, c)
					}
					j++
					continue
				}
				sawEscape = true
				if decoded == nil {
					decoded = make([]byte, 0, len(line)-i)
					decoded = append(decoded, line[rawStart:j]...)
				}
				j++
				if j >= len(line) {
					return nil, fmt.Errorf("unterminated escape")
				}
				esc := line[j]
				switch esc {
				case '\\':
					decoded = append(decoded, '\\')
					j++
				case '"':
					decoded = append(decoded, '"')
					j++
				case 'n':
					decoded = append(decoded, '\n')
					j++
				case 'r':
					decoded = append(decoded, '\r')
					j++
				case 't':
					decoded = append(decoded, '\t')
					j++
				case 'x':
					if j+2 >= len(line) {
						return nil, fmt.Errorf("short \\x escape")
					}
					hi, ok := textHexNibble(line[j+1])
					if !ok {
						return nil, fmt.Errorf("invalid hex char %q", line[j+1])
					}
					lo, ok := textHexNibble(line[j+2])
					if !ok {
						return nil, fmt.Errorf("invalid hex char %q", line[j+2])
					}
					decoded = append(decoded, (hi<<4)|lo)
					j = j + 3
				default:
					return nil, fmt.Errorf("unsupported escape \\%c", esc)
				}
			}
			if j > len(line) || line[j-1] != '"' {
				return nil, fmt.Errorf("unterminated string")
			}
			if sawEscape {
				out = append(out, string(decoded))
			} else {
				out = append(out, line[rawStart:j-1])
			}
			i = j
			continue
		}
		j := i
		for j < len(line) {
			c := line[j]
			if c == ' ' || c == '\t' || c == '\r' || c == '{' || c == '}' || c == '=' || c == '#' {
				break
			}
			j++
		}
		out = append(out, line[i:j])
		i = j
	}
	return out, nil
}

func textHexChar(v byte) byte {
	if v < 10 {
		return '0' + v
	}
	return 'a' + v - 10
}

func parseAttrsInto(tokens []string, attrs map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(attrs))
	i := 0
	for i < len(tokens) {
		if i+2 >= len(tokens) {
			return nil, fmt.Errorf("malformed attrs")
		}
		if tokens[i+1] != "=" {
			return nil, fmt.Errorf("expected '=' after %q", tokens[i])
		}
		out[tokens[i]] = tokens[i+2]
		i = i + 3
	}
	return out, nil
}

func parseTypeRef(s string) (int, error) {
	if !strings.HasPrefix(s, "@") {
		return 0, fmt.Errorf("expected type ref, got %q", s)
	}
	v, err := strconv.Atoi(s[1:])
	if err != nil {
		return 0, fmt.Errorf("invalid type ref %q", s)
	}
	return v, nil
}

func requireAttr(attrs map[string]string, key string) (string, error) {
	v, ok := attrs[key]
	if !ok {
		return "", fmt.Errorf("missing attr %q", key)
	}
	return v, nil
}

func parseIntAttr(attrs map[string]string, key string) (int, error) {
	raw, err := requireAttr(attrs, key)
	if err != nil {
		return 0, err
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid int attr %q=%q", key, raw)
	}
	return v, nil
}

func parseInt64Attr(attrs map[string]string, key string) (int64, error) {
	raw, err := requireAttr(attrs, key)
	if err != nil {
		return 0, err
	}
	v, err := parseInt64Literal(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid int64 attr %q=%q", key, raw)
	}
	return v, nil
}

func parseBoolAttr(attrs map[string]string, key string) (bool, error) {
	raw, err := requireAttr(attrs, key)
	if err != nil {
		return false, err
	}
	if raw == "true" {
		return true, nil
	}
	if raw == "false" {
		return false, nil
	}
	return false, fmt.Errorf("invalid bool attr %q=%q", key, raw)
}

func parseOptionalIntAttr(attrs map[string]string, key string, fallback int) (int, error) {
	raw, ok := attrs[key]
	if !ok {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid int attr %q=%q", key, raw)
	}
	return v, nil
}

func parseOptionalInt64Attr(attrs map[string]string, key string, fallback int64) (int64, error) {
	raw, ok := attrs[key]
	if !ok {
		return fallback, nil
	}
	v, err := parseInt64Literal(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid int64 attr %q=%q", key, raw)
	}
	return v, nil
}

func parseInt64Literal(raw string) (int64, error) {
	if strings.HasPrefix(raw, "0x") || strings.HasPrefix(raw, "0X") {
		if len(raw) == 2 {
			return 0, fmt.Errorf("empty hex literal")
		}
		var u uint64
		for i := 2; i < len(raw); i++ {
			nib, ok := textHexNibble(raw[i])
			if !ok {
				return 0, fmt.Errorf("invalid hex char %q", raw[i])
			}
			u = (u << 4) | uint64(nib)
		}
		return int64(u), nil
	}
	return strconv.ParseInt(raw, 10, 64)
}

func parseOptionalStringAttr(attrs map[string]string, key string, fallback string) string {
	raw, ok := attrs[key]
	if !ok {
		return fallback
	}
	return raw
}

func (d *textDecoder) consumeTop(tokens []string) error {
	if len(tokens) == 2 && tokens[1] == "{" {
		switch tokens[0] {
		case "types", "root_types", "globals", "funcs", "link_static_funcs",
			"zero_call_funcs", "type_ids", "method_table", "iface_methods",
			"iface_method_rets", "callback_funcs":
			d.push(tokens[0])
			return nil
		}
	}
	return fmt.Errorf("unexpected top-level line")
}

func (d *textDecoder) consumeTypes(tokens []string) error {
	switch tokens[0] {
	case "type":
		if len(tokens) < 3 {
			return fmt.Errorf("malformed type line")
		}
		id, err := parseTypeRef(tokens[1])
		if err != nil {
			return err
		}
		attrs, err := parseAttrsInto(tokens[2:], d.attrsTmp)
		if err != nil {
			return err
		}
		if err := d.ensureTypeRecord(id); err != nil {
			return err
		}
		rec := d.types[id]
		kindRaw, err := requireAttr(attrs, "kind")
		if err != nil {
			return err
		}
		kind, ok := typeKindByName[kindRaw]
		if !ok {
			return fmt.Errorf("unknown type kind %q", kindRaw)
		}
		rec.kind = kind
		rec.name, err = requireAttr(attrs, "name")
		if err != nil {
			return err
		}
		rec.pkg, err = requireAttr(attrs, "pkg")
		if err != nil {
			return err
		}
		rec.size, err = parseIntAttr(attrs, "size")
		if err != nil {
			return err
		}
		rec.align, err = parseIntAttr(attrs, "align")
		if err != nil {
			return err
		}
		elemRaw, err := requireAttr(attrs, "elem")
		if err != nil {
			return err
		}
		rec.elemIdx, err = parseTypeRef(elemRaw)
		if err != nil {
			return err
		}
		keyRaw, err := requireAttr(attrs, "key")
		if err != nil {
			return err
		}
		rec.keyIdx, err = parseTypeRef(keyRaw)
		if err != nil {
			return err
		}
		return nil
	case "field":
		attrs, err := parseAttrsInto(tokens[1:], d.attrsTmp)
		if err != nil {
			return err
		}
		ownerRaw, err := requireAttr(attrs, "owner")
		if err != nil {
			return err
		}
		owner, err := parseTypeRef(ownerRaw)
		if err != nil {
			return err
		}
		if owner < 0 || owner >= len(d.types) || d.types[owner] == nil {
			return fmt.Errorf("unknown field owner %s", ownerRaw)
		}
		name, err := requireAttr(attrs, "name")
		if err != nil {
			return err
		}
		typeRaw, err := requireAttr(attrs, "type")
		if err != nil {
			return err
		}
		typeIdx, err := parseTypeRef(typeRaw)
		if err != nil {
			return err
		}
		offset, err := parseIntAttr(attrs, "offset")
		if err != nil {
			return err
		}
		d.types[owner].fields = append(d.types[owner].fields, textFieldRec{
			name:    name,
			typeIdx: typeIdx,
			offset:  offset,
		})
		return nil
	case "param":
		attrs, err := parseAttrsInto(tokens[1:], d.attrsTmp)
		if err != nil {
			return err
		}
		ownerRaw, err := requireAttr(attrs, "owner")
		if err != nil {
			return err
		}
		owner, err := parseTypeRef(ownerRaw)
		if err != nil {
			return err
		}
		if owner < 0 || owner >= len(d.types) || d.types[owner] == nil {
			return fmt.Errorf("unknown param owner %s", ownerRaw)
		}
		typeRaw, err := requireAttr(attrs, "type")
		if err != nil {
			return err
		}
		typeIdx, err := parseTypeRef(typeRaw)
		if err != nil {
			return err
		}
		d.types[owner].params = append(d.types[owner].params, typeIdx)
		return nil
	case "result":
		attrs, err := parseAttrsInto(tokens[1:], d.attrsTmp)
		if err != nil {
			return err
		}
		ownerRaw, err := requireAttr(attrs, "owner")
		if err != nil {
			return err
		}
		owner, err := parseTypeRef(ownerRaw)
		if err != nil {
			return err
		}
		if owner < 0 || owner >= len(d.types) || d.types[owner] == nil {
			return fmt.Errorf("unknown result owner %s", ownerRaw)
		}
		typeRaw, err := requireAttr(attrs, "type")
		if err != nil {
			return err
		}
		typeIdx, err := parseTypeRef(typeRaw)
		if err != nil {
			return err
		}
		d.types[owner].results = append(d.types[owner].results, typeIdx)
		return nil
	}
	return fmt.Errorf("unexpected types line")
}

func (d *textDecoder) consumeRootTypes(tokens []string) error {
	if tokens[0] != "root" {
		return fmt.Errorf("unexpected root_types line")
	}
	attrs, err := parseAttrsInto(tokens[1:], d.attrsTmp)
	if err != nil {
		return err
	}
	typeRaw, err := requireAttr(attrs, "type")
	if err != nil {
		return err
	}
	typeIdx, err := parseTypeRef(typeRaw)
	if err != nil {
		return err
	}
	d.rootTypeIdx = append(d.rootTypeIdx, typeIdx)
	return nil
}

func (d *textDecoder) consumeGlobals(tokens []string) error {
	if tokens[0] != "global" {
		return fmt.Errorf("unexpected globals line")
	}
	attrs, err := parseAttrsInto(tokens[1:], d.attrsTmp)
	if err != nil {
		return err
	}
	name, err := requireAttr(attrs, "name")
	if err != nil {
		return err
	}
	typeRaw, err := requireAttr(attrs, "type")
	if err != nil {
		return err
	}
	typeIdx, err := parseTypeRef(typeRaw)
	if err != nil {
		return err
	}
	index, err := parseIntAttr(attrs, "index")
	if err != nil {
		return err
	}
	d.globals = append(d.globals, textGlobalRec{
		name:    name,
		typeIdx: typeIdx,
		index:   index,
	})
	return nil
}

func (d *textDecoder) consumeFuncs(tokens []string) error {
	if len(tokens) < 5 || tokens[0] != "func" || tokens[len(tokens)-1] != "{" {
		return fmt.Errorf("malformed func header")
	}
	name := tokens[1]
	attrs, err := parseAttrsInto(tokens[2:len(tokens)-1], d.attrsTmp)
	if err != nil {
		return err
	}
	params, err := parseIntAttr(attrs, "params")
	if err != nil {
		return err
	}
	rets, err := parseIntAttr(attrs, "rets")
	if err != nil {
		return err
	}
	rec := &textFuncRec{
		name:   name,
		params: params,
		rets:   rets,
		locals: []textLocalRec{},
		code:   []ir.Inst{},
	}
	d.funcs = append(d.funcs, rec)
	d.curFunc = rec
	d.push("func")
	return nil
}

func (d *textDecoder) consumeFunc(tokens []string) error {
	if len(tokens) == 2 && tokens[1] == "{" {
		switch tokens[0] {
		case "locals", "code", "native":
			if tokens[0] == "native" {
				if d.curFunc.native == nil {
					d.curFunc.native = &ir.NativeFunc{}
				}
			}
			d.push(tokens[0])
			return nil
		}
	}
	return fmt.Errorf("unexpected line inside func block")
}

func (d *textDecoder) consumeLocals(tokens []string) error {
	if tokens[0] != "local" {
		return fmt.Errorf("unexpected locals line")
	}
	attrs, err := parseAttrsInto(tokens[1:], d.attrsTmp)
	if err != nil {
		return err
	}
	name, err := requireAttr(attrs, "name")
	if err != nil {
		return err
	}
	typeRaw, err := requireAttr(attrs, "type")
	if err != nil {
		return err
	}
	typeIdx, err := parseTypeRef(typeRaw)
	if err != nil {
		return err
	}
	index, err := parseIntAttr(attrs, "index")
	if err != nil {
		return err
	}
	is64, err := parseBoolAttr(attrs, "is64")
	if err != nil {
		return err
	}
	width, err := parseIntAttr(attrs, "width")
	if err != nil {
		return err
	}
	d.curFunc.locals = append(d.curFunc.locals, textLocalRec{
		local: ir.IRLocal{
			Name:  name,
			Index: index,
			Is64:  is64,
			Width: width,
		},
		typeIdx: typeIdx,
	})
	return nil
}

func (d *textDecoder) consumeCode(tokens []string) error {
	if len(tokens) == 0 {
		return fmt.Errorf("unexpected code line")
	}
	opRaw := tokens[0]
	op, ok := opcodeByName[opRaw]
	if !ok {
		return fmt.Errorf("unknown opcode %q", opRaw)
	}
	attrs, err := parseAttrsInto(tokens[1:], d.attrsTmp)
	if err != nil {
		return err
	}
	for k := range attrs {
		if k != "arg" && k != "val" && k != "width" && k != "name" {
			return fmt.Errorf("unknown instruction attr %q", k)
		}
	}
	arg, err := parseOptionalIntAttr(attrs, "arg", 0)
	if err != nil {
		return err
	}
	val, err := parseOptionalInt64Attr(attrs, "val", 0)
	if err != nil {
		return err
	}
	width, err := parseOptionalIntAttr(attrs, "width", 0)
	if err != nil {
		return err
	}
	name := parseOptionalStringAttr(attrs, "name", "")
	if opRequiresArg(op) {
		if _, ok := attrs["arg"]; !ok {
			return fmt.Errorf("missing attr %q", "arg")
		}
	}
	if opRequiresVal(op) {
		if _, ok := attrs["val"]; !ok {
			return fmt.Errorf("missing attr %q", "val")
		}
	}
	if opRequiresName(op) {
		if _, ok := attrs["name"]; !ok {
			return fmt.Errorf("missing attr %q", "name")
		}
	}
	d.curFunc.code = append(d.curFunc.code, ir.Inst{
		Op:    op,
		Arg:   arg,
		Width: width,
		Val:   val,
		Name:  name,
	})
	return nil
}

func (d *textDecoder) consumeNative(tokens []string) error {
	if d.curFunc.native == nil {
		d.curFunc.native = &ir.NativeFunc{}
	}
	switch tokens[0] {
	case "arch":
		attrs, err := parseAttrsInto(tokens[1:], d.attrsTmp)
		if err != nil {
			return err
		}
		val, err := requireAttr(attrs, "value")
		if err != nil {
			return err
		}
		d.curFunc.native.Arch = val
		return nil
	case "bytes":
		attrs, err := parseAttrsInto(tokens[1:], d.attrsTmp)
		if err != nil {
			return err
		}
		hexStr, err := requireAttr(attrs, "hex")
		if err != nil {
			return err
		}
		decoded, err := textHexDecode(hexStr)
		if err != nil {
			return err
		}
		d.curFunc.native.Code = decoded
		return nil
	case "fixup":
		attrs, err := parseAttrsInto(tokens[1:], d.attrsTmp)
		if err != nil {
			return err
		}
		kind, err := parseIntAttr(attrs, "kind")
		if err != nil {
			return err
		}
		off, err := parseIntAttr(attrs, "off")
		if err != nil {
			return err
		}
		target, err := requireAttr(attrs, "target")
		if err != nil {
			return err
		}
		d.curFunc.native.Fixups = append(d.curFunc.native.Fixups, ir.NativeFixup{
			Kind:   kind,
			Off:    off,
			Target: target,
		})
		return nil
	}
	return fmt.Errorf("unexpected native line")
}

func (d *textDecoder) consumeMapEntry(block string, tokens []string) error {
	if tokens[0] != "entry" {
		return fmt.Errorf("unexpected %s line", block)
	}
	attrs, err := parseAttrsInto(tokens[1:], d.attrsTmp)
	if err != nil {
		return err
	}
	switch block {
	case "link_static_funcs":
		key, err := requireAttr(attrs, "key")
		if err != nil {
			return err
		}
		value, err := requireAttr(attrs, "value")
		if err != nil {
			return err
		}
		if d.linkStatic == nil {
			d.linkStatic = make(map[string]string)
		}
		d.linkStatic[key] = value
		return nil
	case "zero_call_funcs":
		name, err := requireAttr(attrs, "name")
		if err != nil {
			return err
		}
		if d.zeroCallFuncs == nil {
			d.zeroCallFuncs = make(map[string]bool)
		}
		d.zeroCallFuncs[name] = true
		return nil
	case "type_ids":
		key, err := requireAttr(attrs, "key")
		if err != nil {
			return err
		}
		value, err := parseIntAttr(attrs, "value")
		if err != nil {
			return err
		}
		if d.typeIDs == nil {
			d.typeIDs = make(map[string]int)
		}
		d.typeIDs[key] = value
		return nil
	case "method_table":
		key, err := requireAttr(attrs, "key")
		if err != nil {
			return err
		}
		value, err := requireAttr(attrs, "value")
		if err != nil {
			return err
		}
		if d.methodTable == nil {
			d.methodTable = make(map[string]string)
		}
		d.methodTable[key] = value
		return nil
	case "iface_methods":
		iface, err := requireAttr(attrs, "iface")
		if err != nil {
			return err
		}
		method, err := requireAttr(attrs, "method")
		if err != nil {
			return err
		}
		if d.ifaceMethods == nil {
			d.ifaceMethods = make(map[string][]string)
		}
		d.ifaceMethods[iface] = append(d.ifaceMethods[iface], method)
		return nil
	case "iface_method_rets":
		key, err := requireAttr(attrs, "key")
		if err != nil {
			return err
		}
		value, err := parseIntAttr(attrs, "value")
		if err != nil {
			return err
		}
		if d.ifaceMethodRet == nil {
			d.ifaceMethodRet = make(map[string]int)
		}
		d.ifaceMethodRet[key] = value
		return nil
	case "callback_funcs":
		name, err := requireAttr(attrs, "name")
		if err != nil {
			return err
		}
		if d.callbackFuncs == nil {
			d.callbackFuncs = make(map[string]bool)
		}
		d.callbackFuncs[name] = true
		return nil
	}
	return fmt.Errorf("unexpected block %q", block)
}

func (d *textDecoder) consumeTokens(tokens []string) error {
	if !d.seenHeader {
		if len(tokens) == 2 && tokens[0] == "rtgir" && tokens[1] == "2" {
			d.seenHeader = true
			return nil
		}
		return fmt.Errorf("expected %q header", irTextMagicLine)
	}

	if len(tokens) == 1 && tokens[0] == "}" {
		closed, ok := d.pop()
		if !ok {
			return fmt.Errorf("unexpected '}'")
		}
		if closed == "func" {
			d.curFunc = nil
		}
		return nil
	}

	switch d.top() {
	case "":
		return d.consumeTop(tokens)
	case "types":
		return d.consumeTypes(tokens)
	case "root_types":
		return d.consumeRootTypes(tokens)
	case "globals":
		return d.consumeGlobals(tokens)
	case "funcs":
		return d.consumeFuncs(tokens)
	case "func":
		return d.consumeFunc(tokens)
	case "locals":
		return d.consumeLocals(tokens)
	case "code":
		return d.consumeCode(tokens)
	case "native":
		return d.consumeNative(tokens)
	case "link_static_funcs", "zero_call_funcs", "type_ids", "method_table", "iface_methods", "iface_method_rets", "callback_funcs":
		return d.consumeMapEntry(d.top(), tokens)
	default:
		return fmt.Errorf("internal: unknown parser state %q", d.top())
	}
}

func typesFromDecoder(recs []*textTypeRec) ([]*ir.TypeInfo, error) {
	types := make([]*ir.TypeInfo, len(recs))
	for i := 0; i < len(recs); i++ {
		if recs[i] == nil {
			return nil, fmt.Errorf("missing type definition @%d", i)
		}
		types[i] = &ir.TypeInfo{}
	}
	for i := 0; i < len(recs); i++ {
		src := recs[i]
		dst := types[i]
		dst.Kind = src.kind
		dst.Name = src.name
		dst.Pkg = src.pkg
		dst.Size = src.size
		dst.Align = src.align
		if src.elemIdx >= 0 {
			if src.elemIdx >= len(types) {
				return nil, fmt.Errorf("type @%d elem index out of range: @%d", i, src.elemIdx)
			}
			dst.Elem = types[src.elemIdx]
		}
		if src.keyIdx >= 0 {
			if src.keyIdx >= len(types) {
				return nil, fmt.Errorf("type @%d key index out of range: @%d", i, src.keyIdx)
			}
			dst.Key = types[src.keyIdx]
		}
		fields := make([]ir.FieldInfo, 0, len(src.fields))
		for j := 0; j < len(src.fields); j++ {
			f := src.fields[j]
			var ft *ir.TypeInfo
			if f.typeIdx >= 0 {
				if f.typeIdx >= len(types) {
					return nil, fmt.Errorf("type @%d field %d type index out of range: @%d", i, j, f.typeIdx)
				}
				ft = types[f.typeIdx]
			}
			fields = append(fields, ir.FieldInfo{
				Name:   f.name,
				Type:   ft,
				Offset: f.offset,
			})
		}
		dst.Fields = fields
		params := make([]*ir.TypeInfo, 0, len(src.params))
		for j := 0; j < len(src.params); j++ {
			tidx := src.params[j]
			var pt *ir.TypeInfo
			if tidx >= 0 {
				if tidx >= len(types) {
					return nil, fmt.Errorf("type @%d param %d index out of range: @%d", i, j, tidx)
				}
				pt = types[tidx]
			}
			params = append(params, pt)
		}
		dst.Params = params
		results := make([]*ir.TypeInfo, 0, len(src.results))
		for j := 0; j < len(src.results); j++ {
			tidx := src.results[j]
			var rt *ir.TypeInfo
			if tidx >= 0 {
				if tidx >= len(types) {
					return nil, fmt.Errorf("type @%d result %d index out of range: @%d", i, j, tidx)
				}
				rt = types[tidx]
			}
			results = append(results, rt)
		}
		dst.Results = results
	}
	return types, nil
}

func (d *textDecoder) buildModule() (*ir.IRModule, error) {
	if !d.seenHeader {
		return nil, fmt.Errorf("missing IR text header")
	}
	if len(d.stack) != 0 {
		return nil, fmt.Errorf("unclosed block %q", d.stack[len(d.stack)-1])
	}

	types, err := typesFromDecoder(d.types)
	if err != nil {
		return nil, err
	}

	rootTypes := make([]*ir.TypeInfo, 0, len(d.rootTypeIdx))
	for i := 0; i < len(d.rootTypeIdx); i++ {
		idx := d.rootTypeIdx[i]
		if idx < 0 || idx >= len(types) {
			return nil, fmt.Errorf("root type index out of range: @%d", idx)
		}
		rootTypes = append(rootTypes, types[idx])
	}

	globals := make([]ir.IRGlobal, 0, len(d.globals))
	for i := 0; i < len(d.globals); i++ {
		g := d.globals[i]
		var gt *ir.TypeInfo
		if g.typeIdx >= 0 {
			if g.typeIdx >= len(types) {
				return nil, fmt.Errorf("global %q type index out of range: @%d", g.name, g.typeIdx)
			}
			gt = types[g.typeIdx]
		}
		globals = append(globals, ir.IRGlobal{
			Name:  g.name,
			Type:  gt,
			Index: g.index,
		})
	}

	funcs := make([]*ir.IRFunc, 0, len(d.funcs))
	for i := 0; i < len(d.funcs); i++ {
		src := d.funcs[i]
		fn := &ir.IRFunc{
			Name:     src.name,
			Params:   src.params,
			RetCount: src.rets,
			Locals:   make([]ir.IRLocal, 0, len(src.locals)),
			Code:     src.code,
		}
		for j := 0; j < len(src.locals); j++ {
			l := src.locals[j]
			local := l.local
			if l.typeIdx >= 0 {
				if l.typeIdx >= len(types) {
					return nil, fmt.Errorf("func %q local type index out of range: @%d", src.name, l.typeIdx)
				}
				local.Type = types[l.typeIdx]
			}
			fn.Locals = append(fn.Locals, local)
		}
		if src.native != nil {
			fn.Native = src.native
		}
		funcs = append(funcs, fn)
	}

	return &ir.IRModule{
		Funcs:           funcs,
		Globals:         globals,
		Types:           rootTypes,
		LinkStaticFuncs: d.linkStatic,
		ZeroCallFuncs:   d.zeroCallFuncs,
		TypeIDs:         d.typeIDs,
		MethodTable:     d.methodTable,
		IfaceMethods:    d.ifaceMethods,
		IfaceMethodRets: d.ifaceMethodRet,
		CallbackFuncs:   d.callbackFuncs,
	}, nil
}

func ReadIRText(path string) (*ir.IRModule, error) {
	var in *os.File
	closeInput := false
	if path == "-" {
		in = os.Stdin
	} else {
		var err error
		in, err = os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open file: %w", err)
		}
		closeInput = true
	}

	mod, err := decodeIRTextStream(in)
	if closeInput {
		_ = in.Close()
	}
	if err != nil {
		if path == "-" {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return nil, fmt.Errorf("read file: %w", err)
	}
	return mod, nil
}

func decodeIRTextStream(r *os.File) (*ir.IRModule, error) {
	dec := &textDecoder{
		attrsTmp: make(map[string]string),
	}
	tokenScratch := make([]string, 0, 24)
	readBuf := make([]byte, 4096)
	lineBuf := make([]byte, 0, 256)
	lineNo := 1

	consumeLine := func(line []byte, n int) error {
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		tokens, err := tokenizeTextLineInto(string(line), tokenScratch)
		if err != nil {
			return fmt.Errorf("line %d: tokenize: %w", n, err)
		}
		tokenScratch = tokens[:0]
		if len(tokens) > 0 {
			if err := dec.consumeTokens(tokens); err != nil {
				return fmt.Errorf("line %d: %w", n, err)
			}
		}
		return nil
	}

	for {
		n, err := r.Read(readBuf)
		if n > 0 {
			chunk := readBuf[:n]
			start := 0
			for i := 0; i < len(chunk); i++ {
				if chunk[i] != '\n' {
					continue
				}
				lineBuf = append(lineBuf, chunk[start:i]...)
				if err := consumeLine(lineBuf, lineNo); err != nil {
					return nil, err
				}
				lineBuf = lineBuf[:0]
				lineNo++
				start = i + 1
			}
			if start < len(chunk) {
				lineBuf = append(lineBuf, chunk[start:]...)
			}
		}
		if err != nil {
			if isEOFReadError(err) {
				break
			}
			return nil, err
		}
		if n == 0 {
			break
		}
	}
	if len(lineBuf) > 0 {
		if err := consumeLine(lineBuf, lineNo); err != nil {
			return nil, err
		}
	}
	mod, err := dec.buildModule()
	if err != nil {
		return nil, err
	}
	return mod, nil
}

func isEOFReadError(err error) bool {
	if err == nil {
		return false
	}
	return err == io.EOF
}
