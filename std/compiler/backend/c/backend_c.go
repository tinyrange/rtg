//go:build !no_backend_c

package c

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"j5.nz/rtg/std/compiler/backend/becommon"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

type cDispatchEntry struct {
	typeID   int
	methodID int
	funcID   int
}

func cQuote(s string) string {
	bp := &strings.Builder{}
	bp.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			bp.WriteString("\\\\")
		case '"':
			bp.WriteString("\\\"")
		case '\n':
			bp.WriteString("\\n")
		case '\r':
			bp.WriteString("\\r")
		case '\t':
			bp.WriteString("\\t")
		default:
			if c < 32 || c > 126 {
				hex := "0123456789abcdef"
				bp.WriteString("\\x")
				bp.WriteByte(hex[(c>>4)&0xf])
				bp.WriteByte(hex[c&0xf])
			} else {
				bp.WriteByte(c)
			}
		}
	}
	bp.WriteByte('"')
	return bp.String()
}

func cBareMethod(name string) string {
	dot := -1
	i := len(name) - 1
	for i >= 0 {
		if name[i] == '.' {
			dot = i
			break
		}
		i = i - 1
	}
	if dot < 0 || dot+1 >= len(name) {
		return name
	}
	return name[dot+1:]
}

func cTypeNamesSorted(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func cMethodNamesSorted(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func cModuleNeedsFloatKinds(irmod *ir.IRModule) (bool, bool) {
	needF32 := false
	needF64 := false
	markKind := func(kind ir.TypeKind) {
		if kind == ir.TY_FLOAT32 {
			needF32 = true
		}
		if kind == ir.TY_FLOAT64 {
			needF64 = true
		}
	}
	for _, g := range irmod.Globals {
		if g.Type != nil {
			markKind(g.Type.Kind)
		}
	}
	for _, t := range irmod.Types {
		if t != nil {
			markKind(t.Kind)
		}
	}
	for _, f := range irmod.Funcs {
		for _, k := range f.ResultKinds {
			markKind(k)
		}
		for _, l := range f.Locals {
			markKind(l.FloatKind)
		}
		for _, in := range f.Code {
			switch in.Op {
			case ir.OP_CONST_F32:
				needF32 = true
			case ir.OP_CONST_F64:
				needF64 = true
			}
			if in.Name == "float32" {
				needF32 = true
			}
			if in.Name == "float64" {
				needF64 = true
			}
		}
	}
	return needF32, needF64
}

func cMangleSymbol(name string) string {
	hex := "0123456789abcdef"
	bp := &strings.Builder{}
	bp.WriteString("rtg_fn_")
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			bp.WriteByte(c)
		} else {
			bp.WriteByte('_')
			bp.WriteByte(hex[(c>>4)&0xf])
			bp.WriteByte(hex[c&0xf])
		}
	}
	return bp.String()
}

func cWritef(b *strings.Builder, format string, a ...interface{}) {
	b.WriteString(fmt.Sprintf(format, a...))
}

func cDecimalI64(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	var u uint64
	if neg {
		u = uint64(-(v + 1))
		u = u + 1
	} else {
		u = uint64(v)
	}
	var buf [32]byte
	i := len(buf)
	for u != 0 {
		i--
		buf[i] = byte('0' + (u % 10))
		u = u / 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:len(buf)])
}

func cSignedLiteral(val int64, bits int) string {
	b := &strings.Builder{}
	if bits == 16 && val == -32768 {
		b.WriteByte('(')
		b.WriteByte('-')
		b.WriteString(cDecimalI64(32767))
		b.WriteByte(' ')
		b.WriteByte('-')
		b.WriteByte(' ')
		b.WriteByte('1')
		b.WriteByte(')')
		return b.String()
	}
	if bits == 32 && val == -2147483648 {
		b.WriteByte('(')
		b.WriteByte('-')
		b.WriteString(cDecimalI64(2147483647))
		b.WriteByte('L')
		b.WriteByte(' ')
		b.WriteByte('-')
		b.WriteByte(' ')
		b.WriteByte('1')
		b.WriteByte('L')
		b.WriteByte(')')
		return b.String()
	}
	if bits == 64 && val == -9223372036854775808 {
		b.WriteByte('(')
		b.WriteByte('-')
		b.WriteString(cDecimalI64(9223372036854775807))
		b.WriteByte('L')
		b.WriteByte('L')
		b.WriteByte(' ')
		b.WriteByte('-')
		b.WriteByte(' ')
		b.WriteByte('1')
		b.WriteByte('L')
		b.WriteByte('L')
		b.WriteByte(')')
		return b.String()
	}
	b.WriteString(cDecimalI64(val))
	if bits == 32 {
		b.WriteByte('L')
	} else if bits == 64 {
		b.WriteByte('L')
		b.WriteByte('L')
	}
	return b.String()
}

func cEmitIntrinsicResultCall(bp *strings.Builder, callExpr string) {
	cWritef(bp, "  { rtg_sword rv = %s;\n", callExpr)
	bp.WriteString("    if (rv < 0) { rtg_push(0); rtg_push(0); rtg_push((rtg_word)(-(int)rv)); } else { rtg_push((rtg_word)rv); rtg_push(0); rtg_push(0); } }\n")
}

func cEmitIntrinsicHostCall(bp *strings.Builder, name string) bool {
	switch name {
	case "SysRead":
		cEmitIntrinsicResultCall(bp, "rtg_host_read(locals[0], locals[1], locals[2])")
	case "SysWrite":
		cEmitIntrinsicResultCall(bp, "rtg_host_write(locals[0], locals[1], locals[2])")
	case "SysOpen":
		cEmitIntrinsicResultCall(bp, "rtg_host_open(locals[0], locals[1])")
	case "SysClose":
		cEmitIntrinsicResultCall(bp, "rtg_host_close(locals[0])")
	case "SysStat":
		cEmitIntrinsicResultCall(bp, "rtg_host_stat(locals[0])")
	case "SysMkdir":
		cEmitIntrinsicResultCall(bp, "rtg_host_mkdir(locals[0])")
	case "SysRmdir":
		cEmitIntrinsicResultCall(bp, "rtg_host_rmdir(locals[0])")
	case "SysUnlink":
		cEmitIntrinsicResultCall(bp, "rtg_host_unlink(locals[0])")
	case "SysGetcwd":
		cEmitIntrinsicResultCall(bp, "rtg_host_getcwd(locals[0], locals[1])")
	case "SysExit":
		bp.WriteString("  rtg_host_exit((int)locals[0]);\n")
	case "SysMmap":
		cEmitIntrinsicResultCall(bp, "rtg_host_alloc(locals[1])")
	case "SysGetargc":
		bp.WriteString("  rtg_push((rtg_word)g_argc); rtg_push(0); rtg_push(0);\n")
	case "SysGetargv":
		bp.WriteString("  { int idx = (int)locals[0]; rtg_push(((idx >= g_argc) ? 0 : (rtg_word)(rtg_size)g_argv[idx])); rtg_push(0); rtg_push(0); }\n")
	case "SysGetenv":
		cEmitIntrinsicResultCall(bp, "rtg_host_getenv(locals[0])")
	case "SysOpendir":
		cEmitIntrinsicResultCall(bp, "rtg_host_opendir(locals[0])")
	case "SysReaddir":
		cEmitIntrinsicResultCall(bp, "rtg_host_readdir(locals[0], locals[1], locals[2], 0)")
	case "SysReaddirWithType":
		cEmitIntrinsicResultCall(bp, "rtg_host_readdir(locals[0], locals[1], locals[2], locals[3])")
	case "SysClosedir":
		cEmitIntrinsicResultCall(bp, "rtg_host_closedir(locals[0])")
	case "SysSystem":
		cEmitIntrinsicResultCall(bp, "rtg_host_system(locals[0])")
	case "SysPopen":
		cEmitIntrinsicResultCall(bp, "rtg_host_popen(locals[0])")
	case "SysPclose":
		cEmitIntrinsicResultCall(bp, "rtg_host_pclose(locals[0])")
	case "SysChmod":
		cEmitIntrinsicResultCall(bp, "rtg_host_chmod(locals[0], locals[1])")
	case "SysGetpid":
		bp.WriteString("  rtg_push((rtg_word)rtg_host_getpid()); rtg_push(0); rtg_push(0);\n")
	case "SysNanoTime":
		bp.WriteString("  rtg_push((rtg_word)rtg_host_nano_time());\n")
	default:
		return false
	}
	return true
}

func cEmitSignedBinaryOp(bp *strings.Builder, op ir.Opcode) bool {
	switch op {
	case ir.OP_ADD:
		bp.WriteString("  a = rtg_pop(); c = rtg_pop(); rtg_push((rtg_word)((rtg_sword)c + (rtg_sword)a));\n")
	case ir.OP_SUB:
		bp.WriteString("  a = rtg_pop(); c = rtg_pop(); rtg_push((rtg_word)((rtg_sword)c - (rtg_sword)a));\n")
	case ir.OP_MUL:
		bp.WriteString("  a = rtg_pop(); c = rtg_pop(); rtg_push((rtg_word)((rtg_sword)c * (rtg_sword)a));\n")
	default:
		return false
	}
	return true
}

func cEmitSignedComparePush(bp *strings.Builder, op ir.Opcode) bool {
	var tok string
	switch op {
	case ir.OP_EQ:
		tok = "=="
	case ir.OP_NEQ:
		tok = "!="
	case ir.OP_LT:
		tok = "<"
	case ir.OP_GT:
		tok = ">"
	case ir.OP_LEQ:
		tok = "<="
	case ir.OP_GEQ:
		tok = ">="
	default:
		return false
	}
	cWritef(bp, "  a = rtg_pop(); c = rtg_pop(); rtg_push((rtg_word)(((rtg_sword)c) %s ((rtg_sword)a)));\n", tok)
	return true
}

func cEmitSignedCompareJump(bp *strings.Builder, op ir.Opcode, label int) bool {
	var tok string
	switch op {
	case ir.OP_JMP_EQ:
		tok = "=="
	case ir.OP_JMP_NEQ:
		tok = "!="
	case ir.OP_JMP_LT:
		tok = "<"
	case ir.OP_JMP_GT:
		tok = ">"
	case ir.OP_JMP_LEQ:
		tok = "<="
	case ir.OP_JMP_GEQ:
		tok = ">="
	default:
		return false
	}
	cWritef(bp, "  a = rtg_pop(); c = rtg_pop(); if (((rtg_sword)c) %s ((rtg_sword)a)) goto L_%d;\n", tok, label)
	return true
}

func cFloatBitsHelpers(floatName string) (ctype string, unpack string, pack string, ok bool) {
	switch floatName {
	case "float32":
		return "float", "rtg_bits_f32", "rtg_f32_bits", true
	case "float64":
		return "double", "rtg_bits_f64", "rtg_f64_bits", true
	default:
		return "", "", "", false
	}
}

func cEmitFloatBinaryOp(bp *strings.Builder, op ir.Opcode, floatName string) bool {
	ctype, unpack, pack, ok := cFloatBitsHelpers(floatName)
	if !ok {
		return false
	}
	var tok string
	switch op {
	case ir.OP_ADD:
		tok = "+"
	case ir.OP_SUB:
		tok = "-"
	case ir.OP_MUL:
		tok = "*"
	case ir.OP_DIV:
		tok = "/"
	default:
		return false
	}
	// The IR pushes lhs then rhs, so rtg_pop() yields rhs before lhs.
	cWritef(bp, "  a = rtg_pop(); c = rtg_pop(); rtg_push(%s((%s)%s(c) %s (%s)%s(a)));\n", pack, ctype, unpack, tok, ctype, unpack)
	return true
}

func cEmitFloatComparePush(bp *strings.Builder, op ir.Opcode, floatName string) bool {
	_, unpack, _, ok := cFloatBitsHelpers(floatName)
	if !ok {
		return false
	}
	var tok string
	switch op {
	case ir.OP_EQ:
		tok = "=="
	case ir.OP_NEQ:
		tok = "!="
	case ir.OP_LT:
		tok = "<"
	case ir.OP_GT:
		tok = ">"
	case ir.OP_LEQ:
		tok = "<="
	case ir.OP_GEQ:
		tok = ">="
	default:
		return false
	}
	cWritef(bp, "  a = rtg_pop(); c = rtg_pop(); rtg_push((rtg_word)(%s(c) %s %s(a)));\n", unpack, tok, unpack)
	return true
}

func cEmitFloatCompareJump(bp *strings.Builder, op ir.Opcode, label int, floatName string) bool {
	_, unpack, _, ok := cFloatBitsHelpers(floatName)
	if !ok {
		return false
	}
	var tok string
	switch op {
	case ir.OP_JMP_EQ:
		tok = "=="
	case ir.OP_JMP_NEQ:
		tok = "!="
	case ir.OP_JMP_LT:
		tok = "<"
	case ir.OP_JMP_GT:
		tok = ">"
	case ir.OP_JMP_LEQ:
		tok = "<="
	case ir.OP_JMP_GEQ:
		tok = ">="
	default:
		return false
	}
	cWritef(bp, "  a = rtg_pop(); c = rtg_pop(); if (%s(c) %s %s(a)) goto L_%d;\n", unpack, tok, unpack, label)
	return true
}

const cRuntimeIntrinsicPtr = "  a = locals[0]; rtg_push((a == 0) ? 0 : rtg_load(a, RTG_WORD_BYTES));\n"

const cRuntimeIntrinsicMakeSlice = `  {
    rtg_word h = rtg_alloc((rtg_word)(4 * RTG_WORD_BYTES));
    rtg_store(h + 0 * RTG_WORD_BYTES, locals[0], RTG_WORD_BYTES);
    rtg_store(h + 1 * RTG_WORD_BYTES, locals[1], RTG_WORD_BYTES);
    rtg_store(h + 2 * RTG_WORD_BYTES, locals[2], RTG_WORD_BYTES);
    rtg_store(h + 3 * RTG_WORD_BYTES, 1, RTG_WORD_BYTES);
    rtg_push(h);
  }
`

const cRuntimeIntrinsicMakeString = `  {
    rtg_word h = rtg_alloc((rtg_word)(2 * RTG_WORD_BYTES));
    rtg_store(h + 0 * RTG_WORD_BYTES, locals[0], RTG_WORD_BYTES);
    rtg_store(h + 1 * RTG_WORD_BYTES, locals[1], RTG_WORD_BYTES);
    rtg_push(h);
  }
`

func cEmitRuntimeIntrinsicCall(bp *strings.Builder, name string) bool {
	switch name {
	case "Sliceptr", "Stringptr":
		bp.WriteString(cRuntimeIntrinsicPtr)
	case "Makeslice":
		bp.WriteString(cRuntimeIntrinsicMakeSlice)
	case "Makestring":
		bp.WriteString(cRuntimeIntrinsicMakeString)
	case "Tostring":
		bp.WriteString("  rtg_push(rtg_tostring(locals[0]));\n")
	case "ReadPtr":
		bp.WriteString("  rtg_push(rtg_load(locals[0], RTG_WORD_BYTES));\n")
	case "WritePtr":
		bp.WriteString("  rtg_store(locals[0], locals[1], RTG_WORD_BYTES);\n")
	case "WriteByte":
		bp.WriteString("  rtg_store(locals[0], locals[1], 1);\n")
	default:
		return false
	}
	return true
}

func cEmitSymbolCall(bp *strings.Builder, sym string) {
	bp.WriteString("  ")
	bp.WriteString(sym)
	bp.WriteString("();\n")
}

func cEmitCallOp(bp *strings.Builder, in ir.Inst, funcIdx map[string]int, funcSyms []string) error {
	if strings.HasPrefix(in.Name, "builtin.composite.") {
		cWritef(bp, "  rtg_builtin_composite(%d);\n", in.Arg)
		return nil
	}
	idx, ok := funcIdx[in.Name]
	if !ok {
		return fmt.Errorf("unresolved call target for C backend: %s", in.Name)
	}
	cEmitSymbolCall(bp, funcSyms[idx])
	return nil
}

func cEmitConvertOp(bp *strings.Builder, in ir.Inst, funcSyms []string, bytesToStringIdx int, stringToBytesIdx int) {
	name := in.Name
	floatSrcExpr := ""
	switch in.Val {
	case ir.CONVERT_SRC_FLOAT32:
		floatSrcExpr = "rtg_bits_f32(a)"
	case ir.CONVERT_SRC_FLOAT64:
		floatSrcExpr = "rtg_bits_f64(a)"
	}
	switch name {
	case "string":
		if bytesToStringIdx >= 0 {
			cEmitSymbolCall(bp, funcSyms[bytesToStringIdx])
		}
	case "[]byte":
		if stringToBytesIdx >= 0 {
			cEmitSymbolCall(bp, funcSyms[stringToBytesIdx])
		}
	case "float32":
		switch in.Val {
		case ir.CONVERT_SRC_FLOAT32:
			bp.WriteString("  /* no-op conversion */\n")
		case ir.CONVERT_SRC_FLOAT64:
			bp.WriteString("  a = rtg_pop(); rtg_push(rtg_f32_bits((float)rtg_bits_f64(a)));\n")
		case ir.CONVERT_SRC_UINT:
			bp.WriteString("  a = rtg_pop(); rtg_push(rtg_f32_bits((float)(rtg_word)a));\n")
		default:
			bp.WriteString("  a = rtg_pop(); rtg_push(rtg_f32_bits((float)(rtg_sword)a));\n")
		}
	case "float64":
		switch in.Val {
		case ir.CONVERT_SRC_FLOAT64:
			bp.WriteString("  /* no-op conversion */\n")
		case ir.CONVERT_SRC_FLOAT32:
			bp.WriteString("  a = rtg_pop(); rtg_push(rtg_f64_bits((double)rtg_bits_f32(a)));\n")
		case ir.CONVERT_SRC_UINT:
			bp.WriteString("  a = rtg_pop(); rtg_push(rtg_f64_bits((double)(rtg_word)a));\n")
		default:
			bp.WriteString("  a = rtg_pop(); rtg_push(rtg_f64_bits((double)(rtg_sword)a));\n")
		}
	case "byte", "uint8":
		if floatSrcExpr != "" {
			cWritef(bp, "  a = rtg_pop(); rtg_push((rtg_word)(unsigned char)(%s));\n", floatSrcExpr)
		} else {
			bp.WriteString("  a = rtg_pop(); rtg_push(a & 0xffu);\n")
		}
	case "int8":
		if floatSrcExpr != "" {
			cWritef(bp, "  a = rtg_pop(); rtg_push((rtg_word)(rtg_sword)(signed char)(%s));\n", floatSrcExpr)
		} else {
			bp.WriteString("  a = rtg_pop(); rtg_push((rtg_word)(rtg_sword)(signed char)(unsigned char)a);\n")
		}
	case "uint16":
		if floatSrcExpr != "" {
			cWritef(bp, "  a = rtg_pop(); rtg_push((rtg_word)(rtg_u16)(%s));\n", floatSrcExpr)
		} else {
			bp.WriteString("  a = rtg_pop(); rtg_push(a & 0xffffu);\n")
		}
	case "int16":
		if floatSrcExpr != "" {
			cWritef(bp, "  a = rtg_pop(); rtg_push((rtg_word)(rtg_sword)(rtg_i16)(%s));\n", floatSrcExpr)
		} else {
			bp.WriteString("  a = rtg_pop(); rtg_push((rtg_word)(rtg_sword)(rtg_i16)(rtg_u16)a);\n")
		}
	case "int32":
		if floatSrcExpr != "" {
			cWritef(bp, "  a = rtg_pop(); rtg_push((rtg_word)(rtg_sword)(rtg_i32)(%s));\n", floatSrcExpr)
		} else {
			bp.WriteString("  a = rtg_pop(); rtg_push((rtg_word)(rtg_sword)(rtg_i32)(rtg_u32)a);\n")
		}
	case "uint32":
		if floatSrcExpr != "" {
			cWritef(bp, "  a = rtg_pop(); rtg_push((rtg_word)(rtg_u32)(%s));\n", floatSrcExpr)
		} else {
			bp.WriteString("  a = rtg_pop(); rtg_push((rtg_word)(rtg_u32)a);\n")
		}
	case "int", "int64":
		if floatSrcExpr != "" {
			cWritef(bp, "  a = rtg_pop(); rtg_push((rtg_word)(rtg_sword)(rtg_i64)(%s));\n", floatSrcExpr)
		} else {
			bp.WriteString("  /* no-op conversion */\n")
		}
	case "uint", "uint64", "uintptr":
		if floatSrcExpr != "" {
			cWritef(bp, "  a = rtg_pop(); rtg_push((rtg_word)(rtg_u64)(%s));\n", floatSrcExpr)
		} else {
			bp.WriteString("  /* no-op conversion */\n")
		}
	default:
		bp.WriteString("  /* no-op conversion */\n")
	}
}

const cDefaultHostCore = `#define RTG_FD_MAX 1024
static FILE* rtg_fd_table[RTG_FD_MAX];
static int rtg_fd_hint = 3;

static void rtg_host_init(void) {
  rtg_fd_table[0] = stdin;
  rtg_fd_table[1] = stdout;
  rtg_fd_table[2] = stderr;
}

static void rtg_host_write_str(const char* s, rtg_size n) {
  fwrite(s, 1, n, stderr);
  fflush(stderr);
}

static void rtg_host_exit(int code) {
  exit(code);
}

static rtg_sword rtg_host_read(rtg_word fd, rtg_word buf, rtg_word len) {
  int ifd = (int)fd;
  FILE* f;
  if (ifd < 0 || ifd >= RTG_FD_MAX) return -1;
  f = rtg_fd_table[ifd];
  if (!f) return -1;
  return (rtg_sword)fread((void*)(rtg_size)buf, 1, (rtg_size)len, f);
}

static rtg_sword rtg_host_write(rtg_word fd, rtg_word buf, rtg_word len) {
  int ifd = (int)fd;
  FILE* f;
  rtg_sword n;
  if (ifd < 0 || ifd >= RTG_FD_MAX) return -1;
  f = rtg_fd_table[ifd];
  if (!f) return -1;
  n = (rtg_sword)fwrite((const void*)(rtg_size)buf, 1, (rtg_size)len, f);
  fflush(f);
  return n;
}

static rtg_sword rtg_host_open(rtg_word pathw, rtg_word flags) {
  const char* path = (const char*)(rtg_size)pathw;
  int fl = (int)flags;
  const char* mode;
  FILE* f;
  int fd;
  int i;
  int start;
  if ((fl & 1) && (fl & 64)) mode = "wb";
  else if (fl & 1) mode = "wb";
  else if (fl & 2) mode = "r+b";
  else mode = "rb";
  f = fopen(path, mode);
  if (!f) return -2;
  start = rtg_fd_hint;
  if (start < 3 || start >= RTG_FD_MAX) start = 3;
  fd = -1;
  for (i = start; i < RTG_FD_MAX; i++) { if (!rtg_fd_table[i]) { fd = i; break; } }
  if (fd < 0) {
    for (i = 3; i < start; i++) { if (!rtg_fd_table[i]) { fd = i; break; } }
  }
  if (fd < 0) { fclose(f); return -1; }
  rtg_fd_table[fd] = f;
  rtg_fd_hint = fd + 1;
  if (rtg_fd_hint >= RTG_FD_MAX) rtg_fd_hint = 3;
  return (rtg_sword)fd;
}

static rtg_sword rtg_host_close(rtg_word fdw) {
  int fd = (int)fdw;
  if (fd < 3 || fd >= RTG_FD_MAX || !rtg_fd_table[fd]) return -1;
  fclose(rtg_fd_table[fd]);
  rtg_fd_table[fd] = 0;
  if (fd < rtg_fd_hint) rtg_fd_hint = fd;
  return 0;
}

static rtg_sword rtg_host_stat(rtg_word pathw) {
  const char* path = (const char*)(rtg_size)pathw;
  FILE* f = fopen(path, "rb");
  if (f) { fclose(f); return 0; }
#if !defined(__CC65__)
#ifdef _WIN32
  { DWORD a = GetFileAttributesA(path); if (a != INVALID_FILE_ATTRIBUTES) return 0; }
#else
  { DIR* d = opendir(path); if (d) { closedir(d); return 0; } }
#endif
#endif
  return -2;
}

static rtg_sword rtg_host_mkdir(rtg_word pathw) {
  int rv = rtg_mkdir((const char*)(rtg_size)pathw);
  if (rv != 0) return -17;
  return 0;
}

static rtg_sword rtg_host_chmod(rtg_word pathw, rtg_word mode) {
#ifdef _WIN32
  (void)pathw; (void)mode; return 0;
#else
  int rv = chmod((const char*)(rtg_size)pathw, (int)mode);
  if (rv != 0) return -1;
  return 0;
#endif
}

static rtg_sword rtg_host_getpid(void) {
#ifdef _WIN32
  return (rtg_sword)GetCurrentProcessId();
#else
  return (rtg_sword)getpid();
#endif
}

static rtg_sword rtg_host_nano_time(void) {
#if defined(__CC65__)
  return 0;
#elif defined(_WIN32)
  LARGE_INTEGER freq;
  LARGE_INTEGER counter;
  unsigned long long sec;
  unsigned long long rem;
  if (!QueryPerformanceFrequency(&freq) || freq.QuadPart <= 0) return 0;
  if (!QueryPerformanceCounter(&counter)) return 0;
  sec = (unsigned long long)(counter.QuadPart / freq.QuadPart);
  rem = (unsigned long long)(counter.QuadPart % freq.QuadPart);
  return (rtg_sword)(sec * 1000000000ull + (rem * 1000000000ull) / (unsigned long long)freq.QuadPart);
#else
#if defined(CLOCK_MONOTONIC)
  struct timespec ts;
  if (clock_gettime(CLOCK_MONOTONIC, &ts) == 0) {
    return (rtg_sword)((unsigned long long)ts.tv_sec * 1000000000ull + (unsigned long long)ts.tv_nsec);
  }
#endif
  return 0;
#endif
}

static rtg_sword rtg_host_rmdir(rtg_word pathw) {
  int rv = rtg_rmdir((const char*)(rtg_size)pathw);
  if (rv != 0) return -1;
  return 0;
}

static rtg_sword rtg_host_unlink(rtg_word pathw) {
  int rv = remove((const char*)(rtg_size)pathw);
  if (rv != 0) return -1;
  return 0;
}

static rtg_sword rtg_host_getcwd(rtg_word buf, rtg_word bufsz) {
  char* rv = rtg_getcwd((char*)(rtg_size)buf, (rtg_size)bufsz);
  if (!rv) return -1;
  return (rtg_sword)rtg_strlen((const char*)(rtg_size)buf);
}

static rtg_sword rtg_host_alloc(rtg_word size) {
  rtg_size sz = (rtg_size)size;
  void* p;
  if (sz == 0) sz = 1;
  p = malloc(sz);
  if (!p) return 0;
  memset(p, 0, sz);
  return (rtg_sword)(rtg_size)p;
}

static rtg_sword rtg_host_getenv(rtg_word keyw) {
  char* val = getenv((const char*)(rtg_size)keyw);
  if (!val) return 0;
  return (rtg_sword)(rtg_size)val;
}

`

const cDefaultHostDirOps = `#if !defined(__CC65__)
#ifdef _WIN32
static rtg_sword rtg_host_opendir(rtg_word pathw) {
  const char* path = (const char*)(rtg_size)pathw;
  char pattern[1024];
  HANDLE h;
  WIN32_FIND_DATAA* fd;
  int plen = (int)rtg_strlen(path);
  if (plen + 3 >= 1024) return -1;
  memcpy(pattern, path, plen);
  pattern[plen] = '\\'; pattern[plen+1] = '*'; pattern[plen+2] = 0;
  fd = (WIN32_FIND_DATAA*)malloc(sizeof(WIN32_FIND_DATAA) + sizeof(HANDLE));
  if (!fd) return -1;
  h = FindFirstFileA(pattern, fd);
  if (h == INVALID_HANDLE_VALUE) { free(fd); return -1; }
  *(HANDLE*)((char*)fd + sizeof(WIN32_FIND_DATAA)) = h;
  return (rtg_sword)(rtg_size)fd;
}
static rtg_sword rtg_host_readdir(rtg_word handlew, rtg_word buf, rtg_word bufsz, rtg_word isDirBufW) {
  WIN32_FIND_DATAA* fd = (WIN32_FIND_DATAA*)(rtg_size)handlew;
  char* out = (char*)(rtg_size)buf;
  rtg_size bufLen = (rtg_size)bufsz;
  char* isDirBuf = (char*)(rtg_size)isDirBufW;
  HANDLE h;
  rtg_size nameLen;
  if (!fd) return 0;
  h = *(HANDLE*)((char*)fd + sizeof(WIN32_FIND_DATAA));
  nameLen = rtg_strlen(fd->cFileName);
  if (nameLen == 0) return 0;
  if (nameLen > bufLen) nameLen = bufLen;
  memcpy(out, fd->cFileName, nameLen);
  if (isDirBuf) *isDirBuf = (fd->dwFileAttributes & FILE_ATTRIBUTE_DIRECTORY) ? 1 : 0;
  if (!FindNextFileA(h, fd)) fd->cFileName[0] = 0;
  return (rtg_sword)nameLen;
}
static rtg_sword rtg_host_closedir(rtg_word handlew) {
  WIN32_FIND_DATAA* fd = (WIN32_FIND_DATAA*)(rtg_size)handlew;
  HANDLE h;
  if (!fd) return 0;
  h = *(HANDLE*)((char*)fd + sizeof(WIN32_FIND_DATAA));
  FindClose(h);
  free(fd);
  return 0;
}
#else
static rtg_sword rtg_host_opendir(rtg_word pathw) {
  DIR* d = opendir((const char*)(rtg_size)pathw);
  if (!d) return -1;
  return (rtg_sword)(rtg_size)d;
}
static rtg_sword rtg_host_readdir(rtg_word handlew, rtg_word buf, rtg_word bufsz, rtg_word isDirBufW) {
  DIR* d = (DIR*)(rtg_size)handlew;
  char* out = (char*)(rtg_size)buf;
  rtg_size bufLen = (rtg_size)bufsz;
  char* isDirBuf = (char*)(rtg_size)isDirBufW;
  struct dirent* ent;
  rtg_size nameLen;
  if (!d) return 0;
  ent = readdir(d);
  if (!ent) return 0;
  nameLen = rtg_strlen(ent->d_name);
  if (nameLen > bufLen) nameLen = bufLen;
  memcpy(out, ent->d_name, nameLen);
  if (isDirBuf) *isDirBuf = (ent->d_type == 4) ? 1 : 0;
  return (rtg_sword)nameLen;
}
static rtg_sword rtg_host_closedir(rtg_word handlew) {
  DIR* d = (DIR*)(rtg_size)handlew;
  if (d) closedir(d);
  return 0;
}
#endif
#else
static rtg_sword rtg_host_opendir(rtg_word p) { (void)p; return -1; }
static rtg_sword rtg_host_readdir(rtg_word h, rtg_word b, rtg_word n, rtg_word d) { (void)h;(void)b;(void)n;(void)d; return 0; }
static rtg_sword rtg_host_closedir(rtg_word h) { (void)h; return 0; }
#endif

`

const cDefaultHostProcessOps = `static rtg_sword rtg_host_system(rtg_word cmdw) {
  return (rtg_sword)system((const char*)(rtg_size)cmdw);
}

static rtg_sword rtg_host_popen(rtg_word cmdw) {
  FILE* f = rtg_popen((const char*)(rtg_size)cmdw, "r");
  int fd;
  int i;
  int start;
  if (!f) return -1;
  start = rtg_fd_hint;
  if (start < 3 || start >= RTG_FD_MAX) start = 3;
  fd = -1;
  for (i = start; i < RTG_FD_MAX; i++) { if (!rtg_fd_table[i]) { fd = i; break; } }
  if (fd < 0) {
    for (i = 3; i < start; i++) { if (!rtg_fd_table[i]) { fd = i; break; } }
  }
  if (fd < 0) { rtg_pclose(f); return -1; }
  rtg_fd_table[fd] = f;
  rtg_fd_hint = fd + 1;
  if (rtg_fd_hint >= RTG_FD_MAX) rtg_fd_hint = 3;
  return (rtg_sword)fd;
}

static rtg_sword rtg_host_pclose(rtg_word fdw) {
  int fd = (int)fdw;
  int rv;
  if (fd < 3 || fd >= RTG_FD_MAX || !rtg_fd_table[fd]) return -1;
  rv = rtg_pclose(rtg_fd_table[fd]);
  rtg_fd_table[fd] = 0;
  if (fd < rtg_fd_hint) rtg_fd_hint = fd;
  return (rtg_sword)rv;
}

#endif /* RTG_CUSTOM_HOST */

`

const cHostDispatchAndRuntimeHelpers = `static rtg_sword rtg_host_call(rtg_word num, rtg_word a0, rtg_word a1, rtg_word a2, rtg_word a3, rtg_word a4, rtg_word a5) {
  (void)a4; (void)a5;
  switch ((int)num) {
  case 0:  return rtg_host_read(a0, a1, a2);
  case 1:  return rtg_host_write(a0, a1, a2);
  case 2:  return rtg_host_open(a0, a1);
  case 3:  return rtg_host_close(a0);
  case 4:  return rtg_host_stat(a0);
  case 5:  return rtg_host_mkdir(a0);
  case 6:  return rtg_host_rmdir(a0);
  case 7:  return rtg_host_unlink(a0);
  case 8:  return rtg_host_getcwd(a0, a1);
  case 9:  rtg_host_exit((int)a0); return 0;
  case 10: return rtg_host_alloc(a1);
  case 11: return (rtg_sword)g_argc;
  case 12: return ((int)a0 >= g_argc) ? 0 : (rtg_sword)(rtg_size)g_argv[(int)a0];
  case 13: return rtg_host_getenv(a0);
  case 14: return rtg_host_opendir(a0);
  case 15: return rtg_host_readdir(a0, a1, a2, a3);
  case 16: return rtg_host_closedir(a0);
  case 17: return rtg_host_system(a0);
  case 18: return rtg_host_popen(a0);
  case 19: return rtg_host_pclose(a0);
  case 20: return rtg_host_chmod(a0, a1);
  default: return -1;
  }
}

static void rtg_write_str(const char* s) {
  rtg_host_write_str(s, rtg_strlen(s));
}

static void rtg_check_ptr_bits(void) {
#if !defined(__SIZEOF_POINTER__)
  if (((int)(sizeof(void*) * 8)) != RTG_PTR_BITS) {
    rtg_write_str("rtg pointer-width mismatch: compiler does not expose __SIZEOF_POINTER__; regenerate with matching -T c/<bits> profile\n");
    rtg_host_exit(1);
  }
#endif
}

static void rtg_fail(const char* msg) {
  rtg_write_str("rtg c backend error: ");
  rtg_write_str(msg);
  rtg_write_str("\n");
  rtg_host_exit(1);
}

static void rtg_push(rtg_word v) {
  if (g_sp >= RTG_STACK_MAX) rtg_fail("operand stack overflow");
  g_stack[g_sp++] = v;
}

static rtg_word rtg_pop(void) {
  if (g_sp <= 0) rtg_fail("operand stack underflow");
  return g_stack[--g_sp];
}

static rtg_word rtg_alloc(rtg_word sz) {
  char* p;
  if (sz == 0) sz = 1;
  p = malloc((rtg_size)sz);
  if (!p) rtg_fail("malloc failed");
  return (rtg_word)(rtg_size)p;
}

static rtg_word rtg_load(rtg_word addr, int size) {
  if (addr == 0) return 0;
  if (size == 1) return (rtg_word)(*(unsigned char*)(rtg_size)addr);
  {
    rtg_word v = 0;
    int i;
    unsigned char* p = (unsigned char*)(rtg_size)addr;
    unsigned char* out = (unsigned char*)&v;
    for (i = 0; i < size; i++) out[i] = p[i];
    return v;
  }
}

static void rtg_memzero(rtg_word addr, int n) {
  int i;
  unsigned char* p = (unsigned char*)(rtg_size)addr;
  for (i = 0; i < n; i++) p[i] = 0;
}

static void rtg_store(rtg_word addr, rtg_word v, int size) {
  if (addr == 0) return;
  if (size == 1) { *(unsigned char*)(rtg_size)addr = (unsigned char)(v & 0xffu); return; }
  {
    int i;
    unsigned char* p = (unsigned char*)(rtg_size)addr;
    unsigned char* in = (unsigned char*)&v;
    for (i = 0; i < size; i++) p[i] = in[i];
  }
}

static rtg_word rtg_f32_bits(float v) {
  union { float f; rtg_u32 u; } bits;
  bits.f = v;
  return (rtg_word)bits.u;
}

static float rtg_bits_f32(rtg_word v) {
  union { float f; rtg_u32 u; } bits;
  bits.u = (rtg_u32)v;
  return bits.f;
}

static rtg_word rtg_f64_bits(double v) {
  union { double f; rtg_u64 u; } bits;
  bits.f = v;
  return (rtg_word)bits.u;
}

static double rtg_bits_f64(rtg_word v) {
  union { double f; rtg_u64 u; } bits;
  bits.u = (rtg_u64)v;
  return bits.f;
}

static int rtg_prefix(const char* s, const char* p) {
  while (*p) { if (*s != *p) return 0; s++; p++; }
  return 1;
}

`

const cInterfaceRuntimeHelpers = `static int rtg_find_dispatch(int typeID, int methodID) {
  int i;
  for (i = 0; i < g_dispatch_count; i++) {
    if (g_dispatch[i].type_id == typeID && g_dispatch[i].method_id == methodID) return g_dispatch[i].func_id;
  }
  return -1;
}

static rtg_word rtg_tostring(rtg_word v) {
  rtg_word first;
  rtg_word concrete;
  int fi;
  if (v == 0) return 0;
  if (v < 4096) {
    if (g_int_to_string_idx < 0) return 0;
    rtg_push(v);
    rtg_call_func(g_int_to_string_idx);
    return rtg_pop();
  }
  first = rtg_load(v, RTG_WORD_BYTES);
  if (first >= 256) return v;
  concrete = rtg_load(v + RTG_WORD_BYTES, RTG_WORD_BYTES);
  if (first == 1) {
    if (g_int_to_string_idx < 0) return 0;
    rtg_push(concrete);
    rtg_call_func(g_int_to_string_idx);
    return rtg_pop();
  }
  if (first == 2) return concrete;
  fi = rtg_find_dispatch((int)first, g_error_method_id);
  if (fi >= 0) { rtg_push(concrete); rtg_call_func(fi); return rtg_pop(); }
  fi = rtg_find_dispatch((int)first, g_string_method_id);
  if (fi >= 0) { rtg_push(concrete); rtg_call_func(fi); return rtg_pop(); }
  return 0;
}

static void rtg_builtin_composite(int fieldCount) {
  rtg_word* tmp;
  rtg_word p;
  int i;
  if (fieldCount <= 0) { rtg_push(0); return; }
  tmp = (rtg_word*)malloc((rtg_size)fieldCount * (rtg_size)sizeof(rtg_word));
  if (!tmp) rtg_fail("composite temp alloc failed");
  for (i = 0; i < fieldCount; i++) tmp[i] = rtg_pop();
  p = rtg_alloc((rtg_word)fieldCount * RTG_WORD_BYTES);
  for (i = 0; i < fieldCount; i++) rtg_store(p + (rtg_word)i * RTG_WORD_BYTES, tmp[fieldCount-1-i], RTG_WORD_BYTES);
  rtg_push(p);
  free(tmp);
}

`

const cIfaceCallTemplate = `  {
    int ac = %d;
    rtg_word* argv = 0;
    int k;
    if (ac > 0) { argv = (rtg_word*)malloc((rtg_size)ac * (rtg_size)sizeof(rtg_word)); if (!argv) rtg_fail("iface argv alloc"); }
    for (k = 0; k < ac; k++) argv[k] = rtg_pop();
    a = rtg_pop();
    c = (a == 0) ? 0 : rtg_load(a, RTG_WORD_BYTES);
    t = (a == 0) ? 0 : rtg_load(a + RTG_WORD_BYTES, RTG_WORD_BYTES);
    rtg_push(t);
    for (k = ac - 1; k >= 0; k--) rtg_push(argv[k]);
    if (argv) free(argv);
    i = rtg_find_dispatch((int)c, %d);
    if (i < 0) rtg_fail("interface dispatch failed");
    rtg_call_func(i);
  }
`

const cPanicTemplate = `  a = rtg_pop();
  c = (a == 0) ? 0 : rtg_load(a, RTG_WORD_BYTES);
  if (c < 256) a = rtg_load(a + RTG_WORD_BYTES, RTG_WORD_BYTES);
  c = (a == 0) ? 0 : rtg_load(a, RTG_WORD_BYTES);
  t = (a == 0) ? 0 : rtg_load(a + RTG_WORD_BYTES, RTG_WORD_BYTES);
  if (c != 0 && t != 0) rtg_host_write_str((const char*)(rtg_size)c, (rtg_size)t);
  rtg_host_write_str("\n", 1);
  rtg_host_exit(2);
`

func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	bits := target.CModel
	if bits == 0 {
		bits = 64
	}
	if bits != 16 && bits != 32 && bits != 64 {
		return fmt.Errorf("invalid C profile: %d", bits)
	}
	needF32, needF64 := cModuleNeedsFloatKinds(irmod)
	if needF32 && bits < 32 {
		return fmt.Errorf("c/%d backend does not support float32 values; use c/32 or c/64", bits)
	}
	if needF64 && bits < 64 {
		return fmt.Errorf("c/%d backend does not support float64 values; use c/64", bits)
	}

	wordBytes := bits / 8
	shiftMask := bits - 1
	signedWord := "long long"
	unsignedWord := "unsigned long long"
	i32Type := "int"
	u32Type := "unsigned int"
	if bits == 32 {
		signedWord = "long"
		unsignedWord = "unsigned long"
	} else if bits == 16 {
		signedWord = "int"
		unsignedWord = "unsigned int"
		i32Type = "long"
		u32Type = "unsigned long"
	}

	funcIdx := make(map[string]int)
	for i, f := range irmod.Funcs {
		funcIdx[f.Name] = i
	}
	funcSyms := make([]string, len(irmod.Funcs))
	for i, f := range irmod.Funcs {
		funcSyms[i] = cMangleSymbol(f.Name)
	}
	entryName := ir.EntryFuncName(irmod)
	mainIdx, ok := funcIdx[entryName]
	if !ok {
		return fmt.Errorf("entrypoint %q not found", entryName)
	}

	// String literal interning.
	// Collect into a set first, then sort to make emitted literal IDs stable.
	litSet := make(map[string]bool)
	for _, f := range irmod.Funcs {
		for _, in := range f.Code {
			if in.Op == ir.OP_CONST_STR {
				s := becommon.DecodeStringLiteral(in.Name)
				litSet[s] = true
			}
		}
	}
	var literals []string
	for s := range litSet {
		literals = append(literals, s)
	}
	sort.Strings(literals)
	litIdx := make(map[string]int, len(literals))
	for i, s := range literals {
		litIdx[s] = i
	}

	// Method name interning for interface dispatch.
	// Collect into a set first, then sort to keep method IDs deterministic.
	methodSet := make(map[string]bool)
	for _, f := range irmod.Funcs {
		for _, in := range f.Code {
			if in.Op == ir.OP_IFACE_CALL {
				name := cBareMethod(in.Name)
				methodSet[name] = true
			}
		}
	}
	for mname := range irmod.MethodTable {
		dot := len(mname) - 1
		for dot >= 0 && mname[dot] != '.' {
			dot--
		}
		if dot >= 0 && dot+1 < len(mname) {
			methodSet[mname[dot+1:]] = true
		}
	}
	methodSet["Error"] = true
	methodSet["String"] = true

	var methods []string
	for name := range methodSet {
		methods = append(methods, name)
	}
	sort.Strings(methods)
	methodID := make(map[string]int, len(methods))
	for i, name := range methods {
		methodID[name] = i
	}
	errorMethodID := methodID["Error"]
	stringMethodID := methodID["String"]

	// Build dispatch table: type_id + method_name -> function index.
	var dispatch []cDispatchEntry
	typesSorted := cTypeNamesSorted(irmod.TypeIDs)
	methodsSorted := cMethodNamesSorted(irmod.MethodTable)
	for _, tname := range typesSorted {
		tid := irmod.TypeIDs[tname]
		prefix := tname + "."
		for _, mname := range methodsSorted {
			if !strings.HasPrefix(mname, prefix) {
				continue
			}
			fqn := irmod.MethodTable[mname]
			idx, ok := funcIdx[fqn]
			if !ok {
				continue
			}
			bare := mname[len(prefix):]
			mid, ok := methodID[bare]
			if !ok {
				continue
			}
			dispatch = append(dispatch, cDispatchEntry{
				typeID:   tid,
				methodID: mid,
				funcID:   idx,
			})
		}
	}
	// dispatch is already deterministic: type names and method names are traversed in sorted order.

	bytesToStringIdx := -1
	if idx, ok := funcIdx["runtime.BytesToString"]; ok {
		bytesToStringIdx = idx
	}
	stringToBytesIdx := -1
	if idx, ok := funcIdx["runtime.StringToBytes"]; ok {
		stringToBytesIdx = idx
	}
	intToStringIdx := -1
	if idx, ok := funcIdx["runtime.IntToString"]; ok {
		intToStringIdx = idx
	}

	bp := &strings.Builder{}
	bp.WriteString("/* Generated by rtg -T c. */\n")
	bp.WriteString("#if defined(__SIZE_TYPE__)\n")
	bp.WriteString("typedef __SIZE_TYPE__ rtg_size;\n")
	bp.WriteString("#else\n")
	bp.WriteString("typedef unsigned int rtg_size;\n")
	bp.WriteString("#endif\n")
	cWritef(bp, "#define RTG_PTR_BITS %d\n", bits)
	cWritef(bp, "#define RTG_WORD_BYTES %d\n", wordBytes)
	cWritef(bp, "#define RTG_SHIFT_MASK %d\n", shiftMask)
	bp.WriteString("#if defined(__SIZEOF_POINTER__)\n")
	bp.WriteString("#if (__SIZEOF_POINTER__ * 8) != RTG_PTR_BITS\n")
	bp.WriteString("#if (__SIZEOF_POINTER__ * 8) == 16\n")
	bp.WriteString("#error \"rtg pointer-width mismatch: compiler target is 16-bit; regenerate with -T c/16\"\n")
	bp.WriteString("#elif (__SIZEOF_POINTER__ * 8) == 32\n")
	bp.WriteString("#error \"rtg pointer-width mismatch: compiler target is 32-bit; regenerate with -T c/32\"\n")
	bp.WriteString("#elif (__SIZEOF_POINTER__ * 8) == 64\n")
	bp.WriteString("#error \"rtg pointer-width mismatch: compiler target is 64-bit; regenerate with -T c/64\"\n")
	bp.WriteString("#else\n")
	bp.WriteString("#error \"rtg pointer-width mismatch: compiler target pointer width is not 16/32/64\"\n")
	bp.WriteString("#endif\n")
	bp.WriteString("#endif\n")
	bp.WriteString("#endif\n")
	cWritef(bp, "typedef %s rtg_sword;\n", signedWord)
	cWritef(bp, "typedef %s rtg_word;\n", unsignedWord)
	bp.WriteString("typedef signed long long rtg_i64;\n")
	bp.WriteString("typedef unsigned long long rtg_u64;\n")
	bp.WriteString("typedef signed short rtg_i16;\n")
	bp.WriteString("typedef unsigned short rtg_u16;\n")
	cWritef(bp, "typedef %s rtg_i32;\n", i32Type)
	cWritef(bp, "typedef %s rtg_u32;\n\n", u32Type)
	if bits == 16 {
		bp.WriteString("enum { RTG_STACK_MAX = 4096 };\n")
	} else {
		bp.WriteString("enum { RTG_STACK_MAX = 1 << 20 };\n")
	}
	bp.WriteString("static rtg_word g_stack[RTG_STACK_MAX];\n")
	bp.WriteString("static int g_sp = 0;\n")
	cWritef(bp, "static rtg_word g_globals[%d];\n\n", len(irmod.Globals))

	// argc/argv globals (always needed, even with custom host)
	bp.WriteString("static int g_argc;\n")
	bp.WriteString("static char** g_argv;\n\n")

	bp.WriteString("static rtg_size rtg_strlen(const char* s) {\n")
	bp.WriteString("  rtg_size n = 0;\n")
	bp.WriteString("  while (s[n] != 0) n++;\n")
	bp.WriteString("  return n;\n")
	bp.WriteString("}\n\n")

	// ------ BEGIN: default host implementation, replaceable via RTG_CUSTOM_HOST ------
	bp.WriteString("/*\n")
	bp.WriteString(" * Default host implementation. Define RTG_CUSTOM_HOST before including\n")
	bp.WriteString(" * this file (or compiling it) and provide your own implementations of\n")
	bp.WriteString(" * the rtg_host_* functions for targets like DOS, bare-metal, etc.\n")
	bp.WriteString(" * See the list of required signatures below.\n")
	bp.WriteString(" */\n")
	bp.WriteString("#ifndef RTG_CUSTOM_HOST\n\n")

	// Includes and platform macros
	bp.WriteString("#include <stdio.h>\n")
	bp.WriteString("#include <stdlib.h>\n")
	bp.WriteString("#include <string.h>\n\n")
	bp.WriteString("#ifdef _WIN32\n")
	bp.WriteString("  #include <direct.h>\n")
	bp.WriteString("  #include <windows.h>\n")
	bp.WriteString("  #include <time.h>\n")
	bp.WriteString("  #define rtg_mkdir(p) _mkdir(p)\n")
	bp.WriteString("  #define rtg_rmdir(p) _rmdir(p)\n")
	bp.WriteString("  #define rtg_getcwd(b,n) _getcwd(b,(int)(n))\n")
	bp.WriteString("  #define rtg_popen(c,m) _popen(c,m)\n")
	bp.WriteString("  #define rtg_pclose(f) _pclose(f)\n")
	bp.WriteString("#else\n")
	if bits == 16 {
		bp.WriteString("  #if !defined(__CC65__)\n")
		bp.WriteString("  #include <sys/stat.h>\n")
		bp.WriteString("  #include <unistd.h>\n")
		bp.WriteString("  #include <dirent.h>\n")
		bp.WriteString("  #include <time.h>\n")
		bp.WriteString("  #define rtg_mkdir(p) mkdir(p,0755)\n")
		bp.WriteString("  #define rtg_rmdir(p) rmdir(p)\n")
		bp.WriteString("  #define rtg_getcwd(b,n) getcwd(b,n)\n")
		bp.WriteString("  #define rtg_popen(c,m) popen(c,m)\n")
		bp.WriteString("  #define rtg_pclose(f) pclose(f)\n")
		bp.WriteString("  #else\n")
		bp.WriteString("  #define rtg_mkdir(p) (-1)\n")
		bp.WriteString("  #define rtg_rmdir(p) (-1)\n")
		bp.WriteString("  #define rtg_getcwd(b,n) (NULL)\n")
		bp.WriteString("  #define rtg_popen(c,m) (NULL)\n")
		bp.WriteString("  #define rtg_pclose(f) (-1)\n")
		bp.WriteString("  #endif\n")
	} else {
		bp.WriteString("  #include <sys/stat.h>\n")
		bp.WriteString("  #include <unistd.h>\n")
		bp.WriteString("  #include <dirent.h>\n")
		bp.WriteString("  #include <time.h>\n")
		bp.WriteString("  #define rtg_mkdir(p) mkdir(p,0755)\n")
		bp.WriteString("  #define rtg_rmdir(p) rmdir(p)\n")
		bp.WriteString("  #define rtg_getcwd(b,n) getcwd(b,n)\n")
		bp.WriteString("  #define rtg_popen(c,m) popen(c,m)\n")
		bp.WriteString("  #define rtg_pclose(f) pclose(f)\n")
	}
	bp.WriteString("#endif\n\n")

	bp.WriteString(cDefaultHostCore)
	bp.WriteString(cDefaultHostDirOps)
	bp.WriteString(cDefaultHostProcessOps)

	bp.WriteString(cHostDispatchAndRuntimeHelpers)

	bp.WriteString("struct rtg_strhdr { rtg_word data; rtg_word len; };\n")
	for i, lit := range literals {
		cWritef(bp, "static const unsigned char g_lit_data_%d[] = {", i)
		if len(lit) == 0 {
			bp.WriteString("0")
		} else {
			for j := 0; j < len(lit); j++ {
				if j%16 == 0 {
					bp.WriteString("\n  ")
				}
				cWritef(bp, "%d", int(lit[j]))
				if j+1 < len(lit) {
					bp.WriteString(",")
				}
			}
		}
		bp.WriteString("\n};\n")
		cWritef(bp, "static struct rtg_strhdr g_lit_hdr_%d;\n", i)
	}
	bp.WriteString("\nstatic void rtg_init_literals(void) {\n")
	for i, lit := range literals {
		cWritef(bp, "  g_lit_hdr_%d.data = (rtg_word)(rtg_size)g_lit_data_%d;\n", i, i)
		cWritef(bp, "  g_lit_hdr_%d.len = (rtg_word)%d;\n", i, len(lit))
	}
	bp.WriteString("}\n\n")

	// Forward declarations for direct calls.
	for i := range irmod.Funcs {
		bp.WriteString("static void ")
		bp.WriteString(funcSyms[i])
		bp.WriteString("(void);\n")
	}
	bp.WriteString("\n")

	bp.WriteString("static void rtg_call_func(int idx) {\n")
	bp.WriteString("  switch (idx) {\n")
	for i := range irmod.Funcs {
		cWritef(bp, "    case %d: ", i)
		bp.WriteString(funcSyms[i])
		bp.WriteString("(); return;\n")
	}
	bp.WriteString("    default: rtg_fail(\"bad function index\");\n")
	bp.WriteString("  }\n")
	bp.WriteString("}\n\n")

	cWritef(bp, "static const char* g_method_names[%d] = {\n", len(methods))
	for _, m := range methods {
		cWritef(bp, "  %s,\n", cQuote(m))
	}
	bp.WriteString("};\n")
	cWritef(bp, "static const int g_error_method_id = %d;\n", errorMethodID)
	cWritef(bp, "static const int g_string_method_id = %d;\n", stringMethodID)
	cWritef(bp, "static const int g_int_to_string_idx = %d;\n", intToStringIdx)
	cWritef(bp, "static const int g_dispatch_count = %d;\n", len(dispatch))
	bp.WriteString("static const struct { int type_id; int method_id; int func_id; } g_dispatch[] = {\n")
	for _, d := range dispatch {
		cWritef(bp, "  {%d, %d, %d},\n", d.typeID, d.methodID, d.funcID)
	}
	bp.WriteString("};\n\n")

	bp.WriteString(cInterfaceRuntimeHelpers)

	for fi, f := range irmod.Funcs {
		if target.CompilerDebug && fi%100 == 0 {
			fmt.Fprintf(os.Stderr, "debug: C codegen func %d/%d (%s)\n", fi, len(irmod.Funcs), f.Name)
		}
		funcStart := bp.Len()
		frameSize := len(f.Locals)
		if f.Params > frameSize {
			frameSize = f.Params
		}
		if frameSize <= 0 {
			frameSize = 1
		}
		needA := false
		needC := false
		needT := false
		needI := f.Params > 0
		for _, in := range f.Code {
			switch in.Op {
			case ir.OP_DUP:
				needT = true
			case ir.OP_ADD, ir.OP_SUB, ir.OP_MUL, ir.OP_DIV, ir.OP_MOD, ir.OP_AND, ir.OP_OR, ir.OP_XOR, ir.OP_SHL, ir.OP_SHR,
				ir.OP_EQ, ir.OP_NEQ, ir.OP_LT, ir.OP_GT, ir.OP_LEQ, ir.OP_GEQ:
				needA = true
				needC = true
			case ir.OP_JMP_EQ, ir.OP_JMP_NEQ, ir.OP_JMP_LT, ir.OP_JMP_GT, ir.OP_JMP_LEQ, ir.OP_JMP_GEQ:
				needA = true
				needC = true
			case ir.OP_NEG, ir.OP_NOT, ir.OP_LOAD, ir.OP_OFFSET, ir.OP_LEN, ir.OP_CAP, ir.OP_JMP_IF, ir.OP_JMP_IF_NOT:
				needA = true
			case ir.OP_STORE:
				needA = true
				needC = true
			case ir.OP_INDEX_ADDR:
				needA = true
				needC = true
				needT = true
			case ir.OP_CONVERT:
				if in.Name == "byte" || in.Name == "uint16" || in.Name == "int16" || in.Name == "int32" || in.Name == "uint32" ||
					in.Name == "int" || in.Name == "uint" || in.Name == "int64" || in.Name == "uint64" || in.Name == "uintptr" ||
					in.Name == "float32" || in.Name == "float64" || in.Val == ir.CONVERT_SRC_FLOAT32 || in.Val == ir.CONVERT_SRC_FLOAT64 {
					needA = true
				}
			case ir.OP_IFACE_BOX:
				needA = true
				needC = true
			case ir.OP_IFACE_CALL:
				needA = true
				needC = true
				needT = true
				needI = true
			case ir.OP_PANIC:
				needA = true
				needC = true
				needT = true
			case ir.OP_CALL_INTRINSIC:
				if in.Name == "Sliceptr" || in.Name == "Stringptr" || in.Name == "ReadPtr" || in.Name == "WritePtr" || in.Name == "WriteByte" {
					needA = true
				}
			}
		}

		bp.WriteString("static void ")
		bp.WriteString(funcSyms[fi])
		bp.WriteString("(void) {\n")
		cWritef(bp, "  rtg_word locals[%d];\n", frameSize)
		var temps []string
		if needA {
			temps = append(temps, "a")
		}
		if needC {
			temps = append(temps, "c")
		}
		if needT {
			temps = append(temps, "t")
		}
		if len(temps) > 0 {
			cWritef(bp, "  rtg_word %s;\n", strings.Join(temps, ", "))
		}
		if needI {
			bp.WriteString("  int i;\n")
		}
		cWritef(bp, "  rtg_memzero((rtg_word)(rtg_size)locals, %d * RTG_WORD_BYTES);\n", frameSize)
		if f.Params > 0 {
			cWritef(bp, "  for (i = %d; i >= 0; i--) locals[i] = rtg_pop();\n", f.Params-1)
		}
		i := 0
		for i < len(f.Code) {
			in := f.Code[i]
			if in.Op == ir.OP_DROP {
				drops := 1
				j := i + 1
				for j < len(f.Code) && f.Code[j].Op == ir.OP_DROP {
					drops++
					j++
				}
				if drops == 1 {
					bp.WriteString("  (void)rtg_pop();\n")
				} else {
					cWritef(bp, "  if (g_sp < %d) rtg_fail(\"operand stack underflow\");\n", drops)
					cWritef(bp, "  g_sp -= %d;\n", drops)
				}
				i = j
				continue
			}
			if in.Op == ir.OP_LABEL {
				cWritef(bp, "L_%d:\n", in.Arg)
				i++
				continue
			}
			switch in.Op {
			case ir.OP_CONST_I64:
				cWritef(bp, "  rtg_push((rtg_word)((rtg_sword)%s));\n", cSignedLiteral(in.Val, bits))
			case ir.OP_CONST_F32:
				cWritef(bp, "  rtg_push(rtg_f32_bits((float)(%s)));\n", in.Name)
			case ir.OP_CONST_F64:
				cWritef(bp, "  rtg_push(rtg_f64_bits((double)(%s)));\n", in.Name)
			case ir.OP_CONST_STR:
				lit := becommon.DecodeStringLiteral(in.Name)
				idx := litIdx[lit]
				cWritef(bp, "  rtg_push((rtg_word)(rtg_size)&g_lit_hdr_%d);\n", idx)
			case ir.OP_CONST_BOOL:
				if in.Arg != 0 {
					bp.WriteString("  rtg_push(1);\n")
				} else {
					bp.WriteString("  rtg_push(0);\n")
				}
			case ir.OP_CONST_NIL:
				bp.WriteString("  rtg_push(0);\n")

			case ir.OP_LOCAL_GET:
				cWritef(bp, "  rtg_push(locals[%d]);\n", in.Arg)
			case ir.OP_LOCAL_SET:
				cWritef(bp, "  locals[%d] = rtg_pop();\n", in.Arg)
			case ir.OP_LOCAL_ADD_IMM:
				cWritef(bp, "  locals[%d] = (rtg_word)((rtg_sword)locals[%d] + (rtg_sword)%d);\n", in.Arg, in.Arg, int64(in.Val))
			case ir.OP_LOCAL_ADDR:
				cWritef(bp, "  rtg_push((rtg_word)(rtg_size)&locals[%d]);\n", in.Arg)
			case ir.OP_GLOBAL_GET:
				cWritef(bp, "  rtg_push(g_globals[%d]);\n", in.Arg)
			case ir.OP_GLOBAL_SET:
				cWritef(bp, "  g_globals[%d] = rtg_pop();\n", in.Arg)
			case ir.OP_GLOBAL_ADDR:
				cWritef(bp, "  rtg_push((rtg_word)(rtg_size)&g_globals[%d]);\n", in.Arg)

			case ir.OP_DUP:
				bp.WriteString("  t = rtg_pop(); rtg_push(t); rtg_push(t);\n")

			case ir.OP_ADD:
				if !cEmitFloatBinaryOp(bp, in.Op, in.Name) {
					cEmitSignedBinaryOp(bp, in.Op)
				}
			case ir.OP_SUB:
				if !cEmitFloatBinaryOp(bp, in.Op, in.Name) {
					cEmitSignedBinaryOp(bp, in.Op)
				}
			case ir.OP_MUL:
				if !cEmitFloatBinaryOp(bp, in.Op, in.Name) {
					cEmitSignedBinaryOp(bp, in.Op)
				}
			case ir.OP_DIV:
				if !cEmitFloatBinaryOp(bp, in.Op, in.Name) {
					bp.WriteString("  a = rtg_pop(); c = rtg_pop(); rtg_push((a == 0) ? 0 : (rtg_word)((rtg_sword)c / (rtg_sword)a));\n")
				}
			case ir.OP_MOD:
				bp.WriteString("  a = rtg_pop(); c = rtg_pop(); rtg_push((a == 0) ? 0 : (rtg_word)((rtg_sword)c % (rtg_sword)a));\n")
			case ir.OP_NEG:
				if _, unpack, pack, ok := cFloatBitsHelpers(in.Name); ok {
					cWritef(bp, "  a = rtg_pop(); rtg_push(%s(-%s(a)));\n", pack, unpack)
				} else {
					bp.WriteString("  a = rtg_pop(); rtg_push((rtg_word)(-(rtg_sword)a));\n")
				}
			case ir.OP_AND:
				bp.WriteString("  a = rtg_pop(); c = rtg_pop(); rtg_push(c & a);\n")
			case ir.OP_OR:
				bp.WriteString("  a = rtg_pop(); c = rtg_pop(); rtg_push(c | a);\n")
			case ir.OP_XOR:
				bp.WriteString("  a = rtg_pop(); c = rtg_pop(); rtg_push(c ^ a);\n")
			case ir.OP_SHL:
				bp.WriteString("  a = rtg_pop(); c = rtg_pop(); rtg_push((rtg_word)(c << (a & RTG_SHIFT_MASK)));\n")
			case ir.OP_SHR:
				bp.WriteString("  a = rtg_pop(); c = rtg_pop(); rtg_push((rtg_word)(((rtg_sword)c) >> (a & RTG_SHIFT_MASK)));\n")
			case ir.OP_EQ:
				if !cEmitFloatComparePush(bp, in.Op, in.Name) {
					cEmitSignedComparePush(bp, in.Op)
				}
			case ir.OP_NEQ:
				if !cEmitFloatComparePush(bp, in.Op, in.Name) {
					cEmitSignedComparePush(bp, in.Op)
				}
			case ir.OP_LT:
				if !cEmitFloatComparePush(bp, in.Op, in.Name) {
					cEmitSignedComparePush(bp, in.Op)
				}
			case ir.OP_GT:
				if !cEmitFloatComparePush(bp, in.Op, in.Name) {
					cEmitSignedComparePush(bp, in.Op)
				}
			case ir.OP_LEQ:
				if !cEmitFloatComparePush(bp, in.Op, in.Name) {
					cEmitSignedComparePush(bp, in.Op)
				}
			case ir.OP_GEQ:
				if !cEmitFloatComparePush(bp, in.Op, in.Name) {
					cEmitSignedComparePush(bp, in.Op)
				}
			case ir.OP_NOT:
				bp.WriteString("  a = rtg_pop(); rtg_push((rtg_word)(a == 0));\n")

			case ir.OP_LOAD:
				if in.Val == 0 {
					if in.Arg == 0 {
						bp.WriteString("  a = rtg_pop(); rtg_push(rtg_load(a, RTG_WORD_BYTES));\n")
					} else {
						cWritef(bp, "  a = rtg_pop(); rtg_push(rtg_load(a, %d));\n", in.Arg)
					}
				} else if in.Arg == 0 {
					cWritef(bp, "  a = rtg_pop(); rtg_push(rtg_load(a + (rtg_word)%d, RTG_WORD_BYTES));\n", in.Val)
				} else {
					cWritef(bp, "  a = rtg_pop(); rtg_push(rtg_load(a + (rtg_word)%d, %d));\n", in.Val, in.Arg)
				}
			case ir.OP_STORE:
				if in.Val == 0 {
					if in.Arg == 0 {
						bp.WriteString("  a = rtg_pop(); c = rtg_pop(); rtg_store(a, c, RTG_WORD_BYTES);\n")
					} else {
						cWritef(bp, "  a = rtg_pop(); c = rtg_pop(); rtg_store(a, c, %d);\n", in.Arg)
					}
				} else if in.Arg == 0 {
					cWritef(bp, "  a = rtg_pop(); c = rtg_pop(); rtg_store(a + (rtg_word)%d, c, RTG_WORD_BYTES);\n", in.Val)
				} else {
					cWritef(bp, "  a = rtg_pop(); c = rtg_pop(); rtg_store(a + (rtg_word)%d, c, %d);\n", in.Val, in.Arg)
				}
			case ir.OP_OFFSET:
				cWritef(bp, "  a = rtg_pop(); rtg_push(a + (rtg_word)%d);\n", in.Arg)
			case ir.OP_INDEX_ADDR:
				cWritef(bp, "  a = rtg_pop(); c = rtg_pop(); t = (c == 0) ? 0 : rtg_load(c, RTG_WORD_BYTES); rtg_push(t + a * (rtg_word)%d);\n", in.Arg)
			case ir.OP_LEN:
				bp.WriteString("  a = rtg_pop(); rtg_push((a == 0) ? 0 : rtg_load(a + RTG_WORD_BYTES, RTG_WORD_BYTES));\n")
			case ir.OP_CAP:
				bp.WriteString("  a = rtg_pop(); rtg_push((a == 0) ? 0 : rtg_load(a + 2*RTG_WORD_BYTES, RTG_WORD_BYTES));\n")

			case ir.OP_JMP:
				cWritef(bp, "  goto L_%d;\n", in.Arg)
			case ir.OP_JMP_IF:
				cWritef(bp, "  a = rtg_pop(); if (a != 0) goto L_%d;\n", in.Arg)
			case ir.OP_JMP_IF_NOT:
				cWritef(bp, "  a = rtg_pop(); if (a == 0) goto L_%d;\n", in.Arg)
			case ir.OP_JMP_EQ:
				if !cEmitFloatCompareJump(bp, in.Op, in.Arg, in.Name) {
					cEmitSignedCompareJump(bp, in.Op, in.Arg)
				}
			case ir.OP_JMP_NEQ:
				if !cEmitFloatCompareJump(bp, in.Op, in.Arg, in.Name) {
					cEmitSignedCompareJump(bp, in.Op, in.Arg)
				}
			case ir.OP_JMP_LT:
				if !cEmitFloatCompareJump(bp, in.Op, in.Arg, in.Name) {
					cEmitSignedCompareJump(bp, in.Op, in.Arg)
				}
			case ir.OP_JMP_GT:
				if !cEmitFloatCompareJump(bp, in.Op, in.Arg, in.Name) {
					cEmitSignedCompareJump(bp, in.Op, in.Arg)
				}
			case ir.OP_JMP_LEQ:
				if !cEmitFloatCompareJump(bp, in.Op, in.Arg, in.Name) {
					cEmitSignedCompareJump(bp, in.Op, in.Arg)
				}
			case ir.OP_JMP_GEQ:
				if !cEmitFloatCompareJump(bp, in.Op, in.Arg, in.Name) {
					cEmitSignedCompareJump(bp, in.Op, in.Arg)
				}

			case ir.OP_CALL:
				if err := cEmitCallOp(bp, in, funcIdx, funcSyms); err != nil {
					return err
				}

			case ir.OP_CALL_INTRINSIC:
				if cEmitIntrinsicHostCall(bp, in.Name) {
					break
				}
				if !cEmitRuntimeIntrinsicCall(bp, in.Name) {
					return fmt.Errorf("unknown intrinsic %q", in.Name)
				}

			case ir.OP_RETURN:
				bp.WriteString("  return;\n")

			case ir.OP_CONVERT:
				cEmitConvertOp(bp, in, funcSyms, bytesToStringIdx, stringToBytesIdx)

			case ir.OP_IFACE_BOX:
				cWritef(bp, "  a = rtg_pop(); c = rtg_alloc((rtg_word)(2 * RTG_WORD_BYTES)); rtg_store(c, (rtg_word)%d, RTG_WORD_BYTES); rtg_store(c + RTG_WORD_BYTES, a, RTG_WORD_BYTES); rtg_push(c);\n", in.Arg)

			case ir.OP_IFACE_CALL:
				mid := methodID[cBareMethod(in.Name)]
				cWritef(bp, cIfaceCallTemplate, in.Arg, mid)

			case ir.OP_PANIC:
				bp.WriteString(cPanicTemplate)

			case ir.OP_SLICE_GET, ir.OP_SLICE_MAKE, ir.OP_STRING_GET, ir.OP_STRING_MAKE:
				bp.WriteString("  rtg_fail(\"unexpected unsupported opcode\");\n")

			default:
				return fmt.Errorf("unhandled opcode for C backend: %d", in.Op)
			}
			i++
		}
		bp.WriteString("}\n\n")
		ir.FuncSizes = append(ir.FuncSizes, ir.FuncSize{Name: f.Name, Size: bp.Len() - funcStart})
	}

	bp.WriteString("int main(int argc, char** argv) {\n")
	bp.WriteString("  g_argc = argc;\n")
	bp.WriteString("  g_argv = argv;\n")
	bp.WriteString("  rtg_host_init();\n")
	bp.WriteString("  rtg_check_ptr_bits();\n")
	bp.WriteString("  rtg_init_literals();\n")
	for i, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			cEmitSymbolCall(bp, funcSyms[i])
		}
	}
	cEmitSymbolCall(bp, funcSyms[mainIdx])
	bp.WriteString("  return 0;\n")
	bp.WriteString("}\n")

	if target.CompilerDebug {
		fmt.Fprintf(os.Stderr, "debug: C codegen complete, writing %d bytes\n", bp.Len())
	}
	if err := os.WriteFile(outputPath, []byte(bp.String()), 0644); err != nil {
		return fmt.Errorf("write C source: %v", err)
	}
	return nil
}
