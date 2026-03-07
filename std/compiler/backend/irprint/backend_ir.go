//go:build !no_backend_ir

package irprint

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"j5.nz/rtg/std/compiler/ir"
)

func opcodeName(op ir.Opcode) string {
	switch op {
	case ir.OP_CONST_I64:
		return "const_i64"
	case ir.OP_CONST_F32:
		return "const_f32"
	case ir.OP_CONST_F64:
		return "const_f64"
	case ir.OP_CONST_STR:
		return "const_str"
	case ir.OP_CONST_BOOL:
		return "const_bool"
	case ir.OP_CONST_NIL:
		return "const_nil"
	case ir.OP_LOCAL_GET:
		return "local_get"
	case ir.OP_LOCAL_SET:
		return "local_set"
	case ir.OP_LOCAL_ADD_IMM:
		return "local_add_imm"
	case ir.OP_LOCAL_ADDR:
		return "local_addr"
	case ir.OP_GLOBAL_GET:
		return "global_get"
	case ir.OP_GLOBAL_SET:
		return "global_set"
	case ir.OP_GLOBAL_ADDR:
		return "global_addr"
	case ir.OP_DROP:
		return "drop"
	case ir.OP_DUP:
		return "dup"
	case ir.OP_ADD:
		return "add"
	case ir.OP_SUB:
		return "sub"
	case ir.OP_MUL:
		return "mul"
	case ir.OP_DIV:
		return "div"
	case ir.OP_MOD:
		return "mod"
	case ir.OP_NEG:
		return "neg"
	case ir.OP_AND:
		return "and"
	case ir.OP_OR:
		return "or"
	case ir.OP_XOR:
		return "xor"
	case ir.OP_SHL:
		return "shl"
	case ir.OP_SHR:
		return "shr"
	case ir.OP_EQ:
		return "eq"
	case ir.OP_NEQ:
		return "neq"
	case ir.OP_LT:
		return "lt"
	case ir.OP_GT:
		return "gt"
	case ir.OP_LEQ:
		return "leq"
	case ir.OP_GEQ:
		return "geq"
	case ir.OP_JMP_EQ:
		return "jmp_eq"
	case ir.OP_JMP_NEQ:
		return "jmp_neq"
	case ir.OP_JMP_LT:
		return "jmp_lt"
	case ir.OP_JMP_GT:
		return "jmp_gt"
	case ir.OP_JMP_LEQ:
		return "jmp_leq"
	case ir.OP_JMP_GEQ:
		return "jmp_geq"
	case ir.OP_NOT:
		return "not"
	case ir.OP_LOAD:
		return "load"
	case ir.OP_STORE:
		return "store"
	case ir.OP_OFFSET:
		return "offset"
	case ir.OP_LABEL:
		return "label"
	case ir.OP_JMP:
		return "jmp"
	case ir.OP_JMP_IF:
		return "jmp_if"
	case ir.OP_JMP_IF_NOT:
		return "jmp_if_not"
	case ir.OP_CALL:
		return "call"
	case ir.OP_CALL_INTRINSIC:
		return "call_intrinsic"
	case ir.OP_RETURN:
		return "return"
	case ir.OP_SLICE_GET:
		return "slice_get"
	case ir.OP_SLICE_MAKE:
		return "slice_make"
	case ir.OP_STRING_GET:
		return "string_get"
	case ir.OP_STRING_MAKE:
		return "string_make"
	case ir.OP_INDEX_ADDR:
		return "index_addr"
	case ir.OP_LEN:
		return "len"
	case ir.OP_CAP:
		return "cap"
	case ir.OP_CONVERT:
		return "convert"
	case ir.OP_IFACE_BOX:
		return "iface_box"
	case ir.OP_IFACE_CALL:
		return "iface_call"
	case ir.OP_PANIC:
		return "panic"
	default:
		return fmt.Sprintf("ir.OP_%d", int(op))
	}
}

func typeKindName(k ir.TypeKind) string {
	switch k {
	case ir.TY_VOID:
		return "void"
	case ir.TY_BOOL:
		return "bool"
	case ir.TY_BYTE:
		return "byte"
	case ir.TY_INT32:
		return "int32"
	case ir.TY_INT:
		return "int"
	case ir.TY_FLOAT32:
		return "float32"
	case ir.TY_FLOAT64:
		return "float64"
	case ir.TY_UINTPTR:
		return "uintptr"
	case ir.TY_STRING:
		return "string"
	case ir.TY_POINTER:
		return "pointer"
	case ir.TY_SLICE:
		return "slice"
	case ir.TY_STRUCT:
		return "struct"
	case ir.TY_INTERFACE:
		return "interface"
	case ir.TY_FUNC:
		return "func"
	case ir.TY_MAP:
		return "map"
	default:
		return fmt.Sprintf("type_%d", int(k))
	}
}

func formatType(t *ir.TypeInfo) string {
	if t == nil {
		return "void"
	}
	switch t.Kind {
	case ir.TY_POINTER:
		if t.Elem != nil {
			return "*" + formatType(t.Elem)
		}
		return "*void"
	case ir.TY_SLICE:
		if t.Elem != nil {
			return "[]" + formatType(t.Elem)
		}
		return "[]void"
	case ir.TY_MAP:
		k := "void"
		v := "void"
		if t.Key != nil {
			k = formatType(t.Key)
		}
		if t.Elem != nil {
			v = formatType(t.Elem)
		}
		return "map[" + k + "]" + v
	case ir.TY_STRUCT:
		name := t.Name
		if t.Pkg != "" {
			name = t.Pkg + "." + t.Name
		}
		if name != "" {
			return name
		}
		var sb strings.Builder
		sb.WriteString("struct { ")
		for i, f := range t.Fields {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(f.Name)
			sb.WriteString(": ")
			sb.WriteString(formatType(f.Type))
		}
		sb.WriteString(" }")
		return sb.String()
	case ir.TY_FUNC:
		var sb strings.Builder
		sb.WriteString("func(")
		for i, p := range t.Params {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(formatType(p))
		}
		sb.WriteString(")")
		if len(t.Results) > 0 {
			sb.WriteString(" (")
			for i, r := range t.Results {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(formatType(r))
			}
			sb.WriteString(")")
		}
		return sb.String()
	case ir.TY_INTERFACE:
		name := t.Name
		if t.Pkg != "" {
			name = t.Pkg + "." + t.Name
		}
		if name != "" {
			return name
		}
		return "interface{}"
	default:
		if t.Name != "" {
			if t.Pkg != "" {
				return t.Pkg + "." + t.Name
			}
			return t.Name
		}
		return typeKindName(t.Kind)
	}
}

func irPad4(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n >= 100 {
		return "0" + fmt.Sprintf("%d", n)
	}
	if n >= 10 {
		return "00" + fmt.Sprintf("%d", n)
	}
	return "000" + fmt.Sprintf("%d", n)
}

func irHexByte(c byte) string {
	const hex = "0123456789abcdef"
	return string([]byte{'\\', 'x', hex[c>>4], hex[c&0x0f]})
}

func irQuote(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			sb.WriteString("\\\\")
		case '"':
			sb.WriteString("\\\"")
		case '\n':
			sb.WriteString("\\n")
		case '\r':
			sb.WriteString("\\r")
		case '\t':
			sb.WriteString("\\t")
		default:
			if c < 0x20 || c > 0x7e {
				sb.WriteString(irHexByte(c))
			} else {
				sb.WriteByte(c)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

func Generate(irmod *ir.IRModule, outputPath string) error {
	var sb strings.Builder

	sb.WriteString("; RTG IR Module\n")
	sb.WriteString(fmt.Sprintf("; globals: %d, functions: %d, types: %d\n\n",
		len(irmod.Globals), len(irmod.Funcs), len(irmod.Types)))

	// === Globals ===
	if len(irmod.Globals) > 0 {
		sb.WriteString("; === Globals ===\n")
		for _, g := range irmod.Globals {
			sb.WriteString(fmt.Sprintf("global %d %s : %s\n",
				g.Index, irQuote(g.Name), formatType(g.Type)))
		}
		sb.WriteByte('\n')
	}

	// === Types ===
	if len(irmod.Types) > 0 {
		sb.WriteString("; === Types ===\n")
		for i, t := range irmod.Types {
			sb.WriteString(fmt.Sprintf("type %d %s %s",
				i, irQuote(formatType(t)), typeKindName(t.Kind)))
			if t.Kind == ir.TY_STRUCT && len(t.Fields) > 0 {
				sb.WriteString(" { ")
				for j, f := range t.Fields {
					if j > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(f.Name)
					sb.WriteString(": ")
					sb.WriteString(formatType(f.Type))
				}
				sb.WriteString(" }")
			}
			if t.Size > 0 {
				sb.WriteString(fmt.Sprintf(" size=%d align=%d", t.Size, t.Align))
			}
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}

	// === Type IDs ===
	if len(irmod.TypeIDs) > 0 {
		sb.WriteString("; === Type IDs ===\n")
		typeNames := make([]string, 0, len(irmod.TypeIDs))
		for name := range irmod.TypeIDs {
			typeNames = append(typeNames, name)
		}
		sort.Strings(typeNames)
		for _, name := range typeNames {
			sb.WriteString(fmt.Sprintf("typeid %s = %d\n",
				irQuote(name), irmod.TypeIDs[name]))
		}
		sb.WriteByte('\n')
	}

	// === Method Table ===
	if len(irmod.MethodTable) > 0 {
		sb.WriteString("; === Method Table ===\n")
		methodKeys := make([]string, 0, len(irmod.MethodTable))
		for k := range irmod.MethodTable {
			methodKeys = append(methodKeys, k)
		}
		sort.Strings(methodKeys)
		for _, k := range methodKeys {
			sb.WriteString(fmt.Sprintf("method %s -> %s\n",
				irQuote(k), irQuote(irmod.MethodTable[k])))
		}
		sb.WriteByte('\n')
	}

	// === Interface Methods ===
	if len(irmod.IfaceMethods) > 0 {
		sb.WriteString("; === Interface Methods ===\n")
		ifaceNames := make([]string, 0, len(irmod.IfaceMethods))
		for name := range irmod.IfaceMethods {
			ifaceNames = append(ifaceNames, name)
		}
		sort.Strings(ifaceNames)
		for _, name := range ifaceNames {
			methods := irmod.IfaceMethods[name]
			sb.WriteString(fmt.Sprintf("interface %s { %s }\n",
				irQuote(name), strings.Join(methods, ", ")))
		}
		sb.WriteByte('\n')
	}

	// === Functions ===
	sb.WriteString("; === Functions ===\n")
	for _, f := range irmod.Funcs {
		sb.WriteString(fmt.Sprintf("func %s (params=%d, locals=%d, returns=%d)\n",
			f.Name, f.Params, len(f.Locals), f.RetCount))

		// Local declarations
		for _, l := range f.Locals {
			if l.Type != nil {
				sb.WriteString(fmt.Sprintf("  local %d %s : %s\n",
					l.Index, irQuote(l.Name), formatType(l.Type)))
			} else {
				sb.WriteString(fmt.Sprintf("  local %d %s\n",
					l.Index, irQuote(l.Name)))
			}
		}

		if len(f.Code) > 0 {
			sb.WriteString("  ; body\n")
			for i, inst := range f.Code {
				sb.WriteString("  " + irPad4(i) + ": " + opcodeName(inst.Op) + instArgs(inst, f, irmod) + "\n")
			}
		}
		sb.WriteString("end\n\n")
	}

	return os.WriteFile(outputPath, []byte(sb.String()), 0644)
}

func instArgs(inst ir.Inst, f *ir.IRFunc, irmod *ir.IRModule) string {
	op := inst.Op
	arg := inst.Arg
	val := inst.Val
	name := inst.Name
	w := ""
	if inst.Width != 0 {
		w = " w=" + fmt.Sprintf("%d", inst.Width)
	}
	switch op {
	case ir.OP_CONST_I64:
		return " " + fmt.Sprintf("%d", val) + w
	case ir.OP_CONST_F32, ir.OP_CONST_F64:
		if name != "" {
			return " " + irQuote(name) + w
		}
		return w
	case ir.OP_CONST_STR:
		return " " + irQuote(name)
	case ir.OP_CONST_BOOL:
		if arg != 0 {
			return " true"
		}
		return " false"

	case ir.OP_LOCAL_GET, ir.OP_LOCAL_SET, ir.OP_LOCAL_ADDR:
		s := " " + fmt.Sprintf("%d", arg)
		if arg < len(f.Locals) {
			s = s + "                     ; " + irQuote(f.Locals[arg].Name)
		}
		return s + w
	case ir.OP_LOCAL_ADD_IMM:
		s := " " + fmt.Sprintf("%d", arg) + ", " + fmt.Sprintf("%d", val)
		if arg < len(f.Locals) {
			s = s + "                     ; " + irQuote(f.Locals[arg].Name)
		}
		return s + w

	case ir.OP_GLOBAL_GET, ir.OP_GLOBAL_SET, ir.OP_GLOBAL_ADDR:
		s := " " + fmt.Sprintf("%d", arg)
		if arg >= 0 && arg < len(irmod.Globals) {
			s = s + "                     ; " + irQuote(irmod.Globals[arg].Name)
		}
		return s

	case ir.OP_LABEL, ir.OP_JMP, ir.OP_JMP_IF, ir.OP_JMP_IF_NOT, ir.OP_JMP_EQ, ir.OP_JMP_NEQ, ir.OP_JMP_LT, ir.OP_JMP_GT, ir.OP_JMP_LEQ, ir.OP_JMP_GEQ:
		return " " + fmt.Sprintf("%d", arg)

	case ir.OP_CALL, ir.OP_CALL_INTRINSIC:
		return " " + irQuote(name) + " args=" + fmt.Sprintf("%d", arg)

	case ir.OP_RETURN:
		return " " + fmt.Sprintf("%d", arg)

	case ir.OP_LOAD:
		s := " size=" + fmt.Sprintf("%d", arg)
		if val != 0 {
			s = s + " off=" + fmt.Sprintf("%d", val)
		}
		if name != "" {
			s = s + " kind=" + irQuote(name)
		}
		return s
	case ir.OP_STORE:
		s := " size=" + fmt.Sprintf("%d", arg)
		if val != 0 {
			s = s + " off=" + fmt.Sprintf("%d", val)
		}
		if name != "" {
			s = s + " kind=" + irQuote(name)
		}
		return s
	case ir.OP_OFFSET:
		return " " + fmt.Sprintf("%d", arg)

	case ir.OP_SLICE_GET:
		return " elem_size=" + fmt.Sprintf("%d", arg)
	case ir.OP_INDEX_ADDR:
		return " elem_size=" + fmt.Sprintf("%d", arg)

	case ir.OP_CONVERT:
		if name != "" {
			return " " + irQuote(name)
		}
	case ir.OP_IFACE_BOX:
		if name != "" {
			return " " + irQuote(name)
		}
	case ir.OP_IFACE_CALL:
		s := ""
		if name != "" {
			s = " " + irQuote(name)
		}
		return s + " args=" + fmt.Sprintf("%d", arg)

	case ir.OP_LEN:
		return " kind=" + fmt.Sprintf("%d", arg)
	case ir.OP_CAP:
		return ""
	}
	switch op {
	case ir.OP_ADD, ir.OP_SUB, ir.OP_MUL, ir.OP_DIV, ir.OP_MOD, ir.OP_NEG,
		ir.OP_EQ, ir.OP_NEQ, ir.OP_LT, ir.OP_GT, ir.OP_LEQ, ir.OP_GEQ,
		ir.OP_JMP_EQ, ir.OP_JMP_NEQ, ir.OP_JMP_LT, ir.OP_JMP_GT, ir.OP_JMP_LEQ, ir.OP_JMP_GEQ:
		if name != "" {
			return " kind=" + irQuote(name) + w
		}
	}
	if w != "" {
		return w
	}
	return ""
}
