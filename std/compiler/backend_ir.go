//go:build !no_backend_ir

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func opcodeName(op Opcode) string {
	switch op {
	case OP_CONST_I64:
		return "const_i64"
	case OP_CONST_STR:
		return "const_str"
	case OP_CONST_BOOL:
		return "const_bool"
	case OP_CONST_NIL:
		return "const_nil"
	case OP_LOCAL_GET:
		return "local_get"
	case OP_LOCAL_SET:
		return "local_set"
	case OP_LOCAL_ADDR:
		return "local_addr"
	case OP_GLOBAL_GET:
		return "global_get"
	case OP_GLOBAL_SET:
		return "global_set"
	case OP_GLOBAL_ADDR:
		return "global_addr"
	case OP_DROP:
		return "drop"
	case OP_DUP:
		return "dup"
	case OP_ADD:
		return "add"
	case OP_SUB:
		return "sub"
	case OP_MUL:
		return "mul"
	case OP_DIV:
		return "div"
	case OP_MOD:
		return "mod"
	case OP_NEG:
		return "neg"
	case OP_AND:
		return "and"
	case OP_OR:
		return "or"
	case OP_XOR:
		return "xor"
	case OP_SHL:
		return "shl"
	case OP_SHR:
		return "shr"
	case OP_EQ:
		return "eq"
	case OP_NEQ:
		return "neq"
	case OP_LT:
		return "lt"
	case OP_GT:
		return "gt"
	case OP_LEQ:
		return "leq"
	case OP_GEQ:
		return "geq"
	case OP_NOT:
		return "not"
	case OP_LOAD:
		return "load"
	case OP_STORE:
		return "store"
	case OP_OFFSET:
		return "offset"
	case OP_LABEL:
		return "label"
	case OP_JMP:
		return "jmp"
	case OP_JMP_IF:
		return "jmp_if"
	case OP_JMP_IF_NOT:
		return "jmp_if_not"
	case OP_CALL:
		return "call"
	case OP_CALL_INTRINSIC:
		return "call_intrinsic"
	case OP_RETURN:
		return "return"
	case OP_SLICE_GET:
		return "slice_get"
	case OP_SLICE_MAKE:
		return "slice_make"
	case OP_STRING_GET:
		return "string_get"
	case OP_STRING_MAKE:
		return "string_make"
	case OP_INDEX_ADDR:
		return "index_addr"
	case OP_LEN:
		return "len"
	case OP_CONVERT:
		return "convert"
	case OP_IFACE_BOX:
		return "iface_box"
	case OP_IFACE_CALL:
		return "iface_call"
	case OP_PANIC:
		return "panic"
	default:
		return fmt.Sprintf("op_%d", int(op))
	}
}

func typeKindName(k TypeKind) string {
	switch k {
	case TY_VOID:
		return "void"
	case TY_BOOL:
		return "bool"
	case TY_BYTE:
		return "byte"
	case TY_INT32:
		return "int32"
	case TY_INT:
		return "int"
	case TY_UINTPTR:
		return "uintptr"
	case TY_STRING:
		return "string"
	case TY_POINTER:
		return "pointer"
	case TY_SLICE:
		return "slice"
	case TY_STRUCT:
		return "struct"
	case TY_INTERFACE:
		return "interface"
	case TY_FUNC:
		return "func"
	case TY_MAP:
		return "map"
	default:
		return fmt.Sprintf("type_%d", int(k))
	}
}

func formatType(t *TypeInfo) string {
	if t == nil {
		return "void"
	}
	switch t.Kind {
	case TY_POINTER:
		if t.Elem != nil {
			return "*" + formatType(t.Elem)
		}
		return "*void"
	case TY_SLICE:
		if t.Elem != nil {
			return "[]" + formatType(t.Elem)
		}
		return "[]void"
	case TY_MAP:
		k := "void"
		v := "void"
		if t.Key != nil {
			k = formatType(t.Key)
		}
		if t.Elem != nil {
			v = formatType(t.Elem)
		}
		return "map[" + k + "]" + v
	case TY_STRUCT:
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
	case TY_FUNC:
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
	case TY_INTERFACE:
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

func generateIRText(irmod *IRModule, outputPath string) error {
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
			if t.Kind == TY_STRUCT && len(t.Fields) > 0 {
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
				sb.WriteString("  " + irPad4(i) + ": " + opcodeName(inst.Op) + instArgs(inst.Op, inst.Arg, inst.Val, inst.Name, f, irmod) + "\n")
			}
		}
		sb.WriteString("end\n\n")
	}

	return os.WriteFile(outputPath, []byte(sb.String()), 0644)
}

func instArgs(op Opcode, arg int, val int64, name string, f *IRFunc, irmod *IRModule) string {
	switch op {
	case OP_CONST_I64:
		return " " + fmt.Sprintf("%d", val)
	case OP_CONST_STR:
		return " " + irQuote(name)
	case OP_CONST_BOOL:
		if val != 0 {
			return " true"
		}
		return " false"

	case OP_LOCAL_GET, OP_LOCAL_SET, OP_LOCAL_ADDR:
		s := " " + fmt.Sprintf("%d", arg)
		if arg < len(f.Locals) {
			s = s + "                     ; " + irQuote(f.Locals[arg].Name)
		}
		return s

	case OP_GLOBAL_GET, OP_GLOBAL_SET, OP_GLOBAL_ADDR:
		s := " " + fmt.Sprintf("%d", arg)
		if arg >= 0 && arg < len(irmod.Globals) {
			s = s + "                     ; " + irQuote(irmod.Globals[arg].Name)
		}
		return s

	case OP_LABEL, OP_JMP, OP_JMP_IF, OP_JMP_IF_NOT:
		return " " + fmt.Sprintf("%d", arg)

	case OP_CALL, OP_CALL_INTRINSIC:
		return " " + irQuote(name) + " args=" + fmt.Sprintf("%d", arg)

	case OP_RETURN:
		return " " + fmt.Sprintf("%d", arg)

	case OP_LOAD:
		return " size=" + fmt.Sprintf("%d", arg)
	case OP_STORE:
		return " size=" + fmt.Sprintf("%d", arg)
	case OP_OFFSET:
		return " " + fmt.Sprintf("%d", arg)

	case OP_SLICE_GET:
		return " elem_size=" + fmt.Sprintf("%d", arg)
	case OP_INDEX_ADDR:
		return " elem_size=" + fmt.Sprintf("%d", arg)

	case OP_CONVERT:
		if name != "" {
			return " " + irQuote(name)
		}
	case OP_IFACE_BOX:
		if name != "" {
			return " " + irQuote(name)
		}
	case OP_IFACE_CALL:
		s := ""
		if name != "" {
			s = " " + irQuote(name)
		}
		return s + " args=" + fmt.Sprintf("%d", arg)

	case OP_LEN:
		return " kind=" + fmt.Sprintf("%d", arg)
	}
	return ""
}
