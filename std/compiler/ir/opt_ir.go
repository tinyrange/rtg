package ir

import "j5.nz/rtg/std/compiler/common"

// OptimizeIRModule runs lightweight, backend-independent IR cleanups.
func OptimizeIRModule(target *common.Target, irmod *IRModule) []string {
	return OptimizeIRModuleFrom(target, irmod, 0)
}

// OptimizeIRModuleFrom runs whole-module IR cleanups, but only reapplies the
// per-function cleanup pipeline to funcs starting at optimizeFrom.
func OptimizeIRModuleFrom(target *common.Target, irmod *IRModule, optimizeFrom int) []string {
	var errors []string
	errors = append(errors, inlineZeroCallFuncsFrom(irmod, optimizeFrom)...)
	outlineCompositeLiteralCallsFrom(target, irmod, optimizeFrom)
	funcRetCounts := make(map[string]int, len(irmod.Funcs))
	for _, f := range irmod.Funcs {
		if f == nil {
			continue
		}
		funcRetCounts[f.Name] = f.RetCount
	}
	if optimizeFrom < 0 {
		optimizeFrom = 0
	}
	if optimizeFrom > len(irmod.Funcs) {
		optimizeFrom = len(irmod.Funcs)
	}
	for i := optimizeFrom; i < len(irmod.Funcs); i++ {
		f := irmod.Funcs[i]
		f.Code = optimizeIRFuncCode(target, f, funcRetCounts, irmod.IfaceMethodRets)
	}
	return errors
}

func optimizeIRFuncCode(target *common.Target, f *IRFunc, funcRetCounts map[string]int, ifaceMethodRets map[string]int) []Inst {
	code := f.Code
	if len(code) == 0 {
		return code
	}

	changed := true
	for changed {
		changed = false

		var stepChanged bool
		code, stepChanged = foldNotConditionalJumps(code)
		if stepChanged {
			changed = true
		}

		if !(target.GOOS == "wasi" && target.GOARCH == "wasm32") {
			code, stepChanged = foldConditionalJumpOverUnconditionalJump(code)
			if stepChanged {
				changed = true
			}
		}

		code, stepChanged = foldSliceAppendU32LE(code)
		if stepChanged {
			changed = true
		}

		if !(target.GOOS == "wasi" && target.GOARCH == "wasm32") {
			code, stepChanged = foldOffsetIntoMemoryOps(code)
			if stepChanged {
				changed = true
			}
		}

		if !(target.GOOS == "wasi" && target.GOARCH == "wasm32") {
			code, stepChanged = foldConstAddIntoMemoryOps(code)
			if stepChanged {
				changed = true
			}
		}

		code, stepChanged = annotateNonNilMemoryBases(code, f, funcRetCounts, ifaceMethodRets)
		if stepChanged {
			changed = true
		}

		code, stepChanged = deadLocalStoreToDrop(code, len(f.Locals))
		if stepChanged {
			changed = true
		}

		code, stepChanged = removeUnreachableIRCode(code)
		if stepChanged {
			changed = true
		}

		code, stepChanged = mergeReturnsToSharedEpilogue(code, f.RetCount)
		if stepChanged {
			changed = true
		}

		code, stepChanged = removeRedundantFallthroughJumps(code)
		if stepChanged {
			changed = true
		}

		if !(target.GOOS == "wasi" && target.GOARCH == "wasm32") {
			code, stepChanged = threadJumps(code)
			if stepChanged {
				changed = true
			}
		}

		code, stepChanged = removeUnreferencedLabels(code)
		if stepChanged {
			changed = true
		}
	}

	return code
}

func isBuiltinCompositeCall(inst Inst) bool {
	return inst.Op == OP_CALL && len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite."
}

func compositeHelperName(fieldCount int) string {
	// Keep helper names deterministic and compact without pulling in strconv.
	if fieldCount < 0 {
		fieldCount = -fieldCount
	}
	var buf [16]byte
	i := len(buf)
	if fieldCount == 0 {
		i--
		buf[i] = '0'
	} else {
		v := fieldCount
		for v > 0 {
			i--
			buf[i] = common.HexDigit(byte(v & 0xF))
			v = v >> 4
		}
	}
	return "runtime.$rtgComposite$" + string(buf[i:len(buf)])
}

func targetWordSize(target *common.Target) int {
	if target != nil {
		if target.WordSize > 0 {
			return target.WordSize
		}
		if target.PtrSize > 0 {
			return target.PtrSize
		}
	}
	return 8
}

func buildCompositeHelperFunc(name string, fieldCount int, wordSize int) *IRFunc {
	// Layout: N params (field values) + 1 local for allocated pointer.
	locals := make([]IRLocal, fieldCount+1)
	for i := 0; i < fieldCount; i++ {
		locals[i] = IRLocal{Name: "$f", Index: i, Width: 0}
	}
	ptrLocal := fieldCount
	locals[ptrLocal] = IRLocal{Name: "$ptr", Index: ptrLocal, Width: 0}

	structBytes := fieldCount * wordSize
	code := make([]Inst, 0, 4+fieldCount*4)
	code = append(code, makeInst(OP_CONST_I64, 0, 0, int64(structBytes), ""))
	code = append(code, makeInst(OP_CALL, 1, 0, 0, "runtime.Alloc"))
	code = append(code, makeInst(OP_LOCAL_SET, ptrLocal, 0, 0, ""))

	for i := 0; i < fieldCount; i++ {
		code = append(code, makeInst(OP_LOCAL_GET, i, 0, 0, ""))        // value
		code = append(code, makeInst(OP_LOCAL_GET, ptrLocal, 0, 0, "")) // base addr
		if i != 0 {
			code = append(code, makeInst(OP_OFFSET, i*wordSize, 0, 0, ""))
		}
		code = append(code, makeInst(OP_STORE, wordSize, 0, 0, ""))
	}

	code = append(code, makeInst(OP_LOCAL_GET, ptrLocal, 0, 0, ""))
	code = append(code, makeInst(OP_RETURN, 1, 0, 0, ""))

	return &IRFunc{
		Name:     name,
		Params:   fieldCount,
		Locals:   locals,
		RetCount: 1,
		Code:     code,
	}
}

// outlineCompositeLiteralCalls replaces repeated synthetic composite callsites:
//
//	CALL builtin.composite.<Type> (N fields)
//
// with shared per-arity helpers:
//
//	CALL runtime.$rtgComposite$NN
//
// so backends do not duplicate large constructor lowering at every callsite.
func outlineCompositeLiteralCalls(target *common.Target, irmod *IRModule) {
	outlineCompositeLiteralCallsFrom(target, irmod, 0)
}

func outlineCompositeLiteralCallsFrom(target *common.Target, irmod *IRModule, callerStart int) {
	if irmod == nil || len(irmod.Funcs) == 0 {
		return
	}
	if target != nil && target.GOOS == "wasi" && target.GOARCH == "wasm32" {
		// The wasm32 backend needs the original composite type name so it can
		// recover 8-byte float/int field widths from the operand types.
		return
	}

	const minSitesToOutline = 2
	counts := make(map[int]int)
	existing := make(map[string]bool)
	if callerStart < 0 {
		callerStart = 0
	}
	if callerStart > len(irmod.Funcs) {
		callerStart = len(irmod.Funcs)
	}
	for i, f := range irmod.Funcs {
		existing[f.Name] = true
		if i < callerStart {
			continue
		}
		for _, inst := range f.Code {
			if isBuiltinCompositeCall(inst) && inst.Arg > 0 {
				counts[inst.Arg] = counts[inst.Arg] + 1
			}
		}
	}
	if len(counts) == 0 {
		return
	}

	// Deterministic arity order (insertion sort keeps code small).
	var arities []int
	for arity, count := range counts {
		if count < minSitesToOutline {
			continue
		}
		arities = append(arities, arity)
	}
	for i := 1; i < len(arities); i++ {
		j := i
		for j > 0 && arities[j] < arities[j-1] {
			arities[j], arities[j-1] = arities[j-1], arities[j]
			j--
		}
	}
	if len(arities) == 0 {
		return
	}

	wordSize := targetWordSize(target)
	rewrite := make(map[int]string)
	for _, arity := range arities {
		name := compositeHelperName(arity)
		rewrite[arity] = name
		if existing[name] {
			continue
		}
		irmod.Funcs = append(irmod.Funcs, buildCompositeHelperFunc(name, arity, wordSize))
		existing[name] = true
	}

	for i := callerStart; i < len(irmod.Funcs); i++ {
		f := irmod.Funcs[i]
		for j := range f.Code {
			inst := f.Code[j]
			if !isBuiltinCompositeCall(inst) || inst.Arg <= 0 {
				continue
			}
			name, ok := rewrite[inst.Arg]
			if !ok || name == "" {
				continue
			}
			f.Code[j].Name = name
		}
	}
}

// foldLocalAddImm rewrites:
//
//	LOCAL_GET n; CONST_I64 k; ADD; LOCAL_SET n
//	LOCAL_GET n; CONST_I64 k; SUB; LOCAL_SET n
//
// to:
//
//	LOCAL_ADD_IMM n, (+/-k)
//
// when the local is word-sized.
func foldLocalAddImm(code []Inst, locals []IRLocal) ([]Inst, bool) {
	if len(code) < 4 {
		return code, false
	}
	changed := false
	out := make([]Inst, 0, len(code))
	i := 0
	for i < len(code) {
		if i+3 < len(code) &&
			code[i].Op == OP_LOCAL_GET &&
			code[i+1].Op == OP_CONST_I64 &&
			(code[i+2].Op == OP_ADD || code[i+2].Op == OP_SUB) &&
			code[i+3].Op == OP_LOCAL_SET &&
			code[i].Arg == code[i+3].Arg {

			idx := code[i].Arg
			if idx >= 0 && idx < len(locals) {
				// Keep semantics simple: only fold word-sized locals.
				if locals[idx].Width == 0 {
					imm := code[i+1].Val
					if code[i+2].Op == OP_SUB {
						imm = -imm
					}
					if imm >= -2147483648 && imm <= 2147483647 {
						out = append(out, makeInst(OP_LOCAL_ADD_IMM, idx, 0, imm, ""))
						changed = true
						i += 4
						continue
					}
				}
			}
		}
		out = append(out, code[i])
		i++
	}
	return out, changed
}

// foldNotConditionalJumps rewrites:
//
//	NOT; JMP_IF L     -> JMP_IF_NOT L
//	NOT; JMP_IF_NOT L -> JMP_IF L
//
// when adjacent in the instruction stream.
func foldNotConditionalJumps(code []Inst) ([]Inst, bool) {
	if len(code) < 2 {
		return code, false
	}

	changed := false
	out := make([]Inst, 0, len(code))
	for i := 0; i < len(code); i++ {
		if i+1 < len(code) && code[i].Op == OP_NOT {
			next := code[i+1]
			if next.Op == OP_JMP_IF {
				next.Op = OP_JMP_IF_NOT
				out = append(out, next)
				i++
				changed = true
				continue
			}
			if next.Op == OP_JMP_IF_NOT {
				next.Op = OP_JMP_IF
				out = append(out, next)
				i++
				changed = true
				continue
			}
		}
		out = append(out, code[i])
	}
	return out, changed
}

// foldConditionalJumpOverUnconditionalJump rewrites:
//
//	JCC L_then
//	JMP L_else
//	LABEL L_then
//
// into:
//
//	J!CC L_else
//	LABEL L_then
//
// when the conditional target is the immediate fallthrough label.
func foldConditionalJumpOverUnconditionalJump(code []Inst) ([]Inst, bool) {
	if len(code) < 3 {
		return code, false
	}

	changed := false
	out := make([]Inst, 0, len(code))
	i := 0
	for i < len(code) {
		if i+2 < len(code) && code[i+1].Op == OP_JMP && code[i+2].Op == OP_LABEL && code[i].Arg == code[i+2].Arg {
			inv, ok := invertConditionalJumpOpcode(code[i].Op)
			if ok {
				inst := code[i]
				inst.Op = inv
				inst.Arg = code[i+1].Arg
				out = append(out, inst)
				i += 2
				changed = true
				continue
			}
		}
		out = append(out, code[i])
		i++
	}
	return out, changed
}

func invertConditionalJumpOpcode(op Opcode) (Opcode, bool) {
	switch op {
	case OP_JMP_IF:
		return OP_JMP_IF_NOT, true
	case OP_JMP_IF_NOT:
		return OP_JMP_IF, true
	case OP_JMP_EQ:
		return OP_JMP_NEQ, true
	case OP_JMP_NEQ:
		return OP_JMP_EQ, true
	case OP_JMP_LT:
		return OP_JMP_GEQ, true
	case OP_JMP_GEQ:
		return OP_JMP_LT, true
	case OP_JMP_GT:
		return OP_JMP_LEQ, true
	case OP_JMP_LEQ:
		return OP_JMP_GT, true
	default:
		return 0, false
	}
}

func matchesSliceAppendU64LEWindow(code []Inst, i int) bool {
	// byte(v)
	v0 := code[i]
	if v0.Op != OP_LOCAL_GET {
		return false
	}
	inst1 := code[i+1]
	inst2 := code[i+2]
	inst3 := code[i+3]
	if inst1.Op != OP_CONVERT || inst1.Name != "byte" || inst2.Op != OP_CONST_I64 || inst2.Val != 1 || inst3.Op != OP_CALL || inst3.Name != "runtime.SliceAppend" || inst3.Arg != 3 {
		return false
	}

	shifts := [7]int64{8, 16, 24, 32, 40, 48, 56}
	pos := 4
	for _, shift := range shifts {
		lget := code[i+pos]
		cshift := code[i+pos+1]
		shr := code[i+pos+2]
		cvt := code[i+pos+3]
		cone := code[i+pos+4]
		call := code[i+pos+5]
		if lget.Op != OP_LOCAL_GET || lget.Arg != v0.Arg || lget.Width != v0.Width ||
			cshift.Op != OP_CONST_I64 || cshift.Val != shift ||
			shr.Op != OP_SHR ||
			cvt.Op != OP_CONVERT || cvt.Name != "byte" ||
			cone.Op != OP_CONST_I64 || cone.Val != 1 ||
			call.Op != OP_CALL || call.Name != "runtime.SliceAppend" || call.Arg != 3 {
			return false
		}
		pos += 6
	}

	return true
}

// foldSliceAppendU32LE rewrites a common append-byte pattern:
//
//	append(dst, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
//
// emitted as four runtime.SliceAppend calls into a single:
//
//	runtime.SliceAppendU32LE(dst, v)
//
// The matcher is intentionally strict: it only folds when v is loaded from
// the same local for all four byte extractions.
func foldSliceAppendU32LE(code []Inst) ([]Inst, bool) {
	if len(code) < 22 {
		return code, false
	}

	changed := false
	var out []Inst
	i := 0
	for i < len(code) {
		if i+21 < len(code) && matchesSliceAppendU32LEWindow(code, i) {
			v := code[i]
			if !changed {
				out = make([]Inst, 0, len(code))
				out = append(out, code[:i]...)
			}
			out = append(out, v)
			out = append(out, makeInst(OP_CALL, 2, 0, 0, "runtime.SliceAppendU32LE"))
			changed = true
			i += 22
			continue
		}
		if changed {
			out = append(out, code[i])
		}
		i++
	}
	if !changed {
		return code, false
	}
	return out, changed
}

func matchesSliceAppendU32LEWindow(code []Inst, i int) bool {
	// byte(v)
	v0 := code[i]
	if v0.Op != OP_LOCAL_GET {
		return false
	}
	inst1 := code[i+1]
	inst2 := code[i+2]
	inst3 := code[i+3]
	if inst1.Op != OP_CONVERT || inst1.Name != "byte" || inst2.Op != OP_CONST_I64 || inst2.Val != 1 || inst3.Op != OP_CALL || inst3.Name != "runtime.SliceAppend" || inst3.Arg != 3 {
		return false
	}

	// byte(v >> 8)
	lget4 := code[i+4]
	c8 := code[i+5]
	shr6 := code[i+6]
	cvt7 := code[i+7]
	cone8 := code[i+8]
	call9 := code[i+9]
	if lget4.Op != OP_LOCAL_GET || lget4.Arg != v0.Arg || lget4.Width != v0.Width ||
		c8.Op != OP_CONST_I64 || c8.Val != 8 ||
		shr6.Op != OP_SHR ||
		cvt7.Op != OP_CONVERT || cvt7.Name != "byte" ||
		cone8.Op != OP_CONST_I64 || cone8.Val != 1 ||
		call9.Op != OP_CALL || call9.Name != "runtime.SliceAppend" || call9.Arg != 3 {
		return false
	}

	// byte(v >> 16)
	lget10 := code[i+10]
	c16 := code[i+11]
	shr12 := code[i+12]
	cvt13 := code[i+13]
	cone14 := code[i+14]
	call15 := code[i+15]
	if lget10.Op != OP_LOCAL_GET || lget10.Arg != v0.Arg || lget10.Width != v0.Width ||
		c16.Op != OP_CONST_I64 || c16.Val != 16 ||
		shr12.Op != OP_SHR ||
		cvt13.Op != OP_CONVERT || cvt13.Name != "byte" ||
		cone14.Op != OP_CONST_I64 || cone14.Val != 1 ||
		call15.Op != OP_CALL || call15.Name != "runtime.SliceAppend" || call15.Arg != 3 {
		return false
	}

	// byte(v >> 24)
	lget16 := code[i+16]
	c24 := code[i+17]
	shr18 := code[i+18]
	cvt19 := code[i+19]
	cone20 := code[i+20]
	call21 := code[i+21]
	if lget16.Op != OP_LOCAL_GET || lget16.Arg != v0.Arg || lget16.Width != v0.Width ||
		c24.Op != OP_CONST_I64 || c24.Val != 24 ||
		shr18.Op != OP_SHR ||
		cvt19.Op != OP_CONVERT || cvt19.Name != "byte" ||
		cone20.Op != OP_CONST_I64 || cone20.Val != 1 ||
		call21.Op != OP_CALL || call21.Name != "runtime.SliceAppend" || call21.Arg != 3 {
		return false
	}

	return true
}

// foldOffsetIntoMemoryOps rewrites:
//
//	OFFSET k; LOAD size
//	OFFSET k; STORE size
//
// into:
//
//	LOAD size (with Val += k)
//	STORE size (with Val += k)
//
// so backends can directly use base+immediate addressing forms.
func foldOffsetIntoMemoryOps(code []Inst) ([]Inst, bool) {
	if len(code) < 2 {
		return code, false
	}

	changed := false
	out := make([]Inst, 0, len(code))
	i := 0
	for i < len(code) {
		if i+1 < len(code) && code[i].Op == OP_OFFSET && (code[i+1].Op == OP_LOAD || code[i+1].Op == OP_STORE) {
			next := code[i+1]
			delta := int64(code[i].Arg)
			sum := next.Val + delta
			// Keep behavior explicit; skip pathological int64 overflow cases.
			if (delta > 0 && sum < next.Val) || (delta < 0 && sum > next.Val) {
				out = append(out, code[i])
				i++
				continue
			}
			next.Val = sum
			out = append(out, next)
			changed = true
			i += 2
			continue
		}
		out = append(out, code[i])
		i++
	}
	if !changed {
		return code, false
	}
	return out, true
}

// foldConstAddIntoMemoryOps rewrites:
//
//	CONST_I64 k; ADD; LOAD size
//	CONST_I64 k; ADD; STORE size
//
// into:
//
//	LOAD size (with Val += k)
//	STORE size (with Val += k)
//
// when the ADD result is consumed immediately as a memory address. This covers
// address materialization patterns that were not first canonicalized into
// OP_OFFSET.
func foldConstAddIntoMemoryOps(code []Inst) ([]Inst, bool) {
	if len(code) < 3 {
		return code, false
	}

	changed := false
	out := make([]Inst, 0, len(code))
	i := 0
	for i < len(code) {
		if i+2 < len(code) &&
			code[i].Op == OP_CONST_I64 &&
			code[i+1].Op == OP_ADD &&
			(code[i+2].Op == OP_LOAD || code[i+2].Op == OP_STORE) {
			next := code[i+2]
			delta := code[i].Val
			sum := next.Val + delta
			// Keep behavior explicit; skip pathological int64 overflow cases.
			if (delta > 0 && sum < next.Val) || (delta < 0 && sum > next.Val) {
				out = append(out, code[i])
				i++
				continue
			}
			next.Val = sum
			out = append(out, next)
			changed = true
			i += 3
			continue
		}
		out = append(out, code[i])
		i++
	}
	if !changed {
		return code, false
	}
	return out, true
}

type nonNilState struct {
	locals []bool
	stack  []bool
}

func cloneNonNilState(src *nonNilState) *nonNilState {
	if src == nil {
		return nil
	}
	dst := &nonNilState{
		locals: make([]bool, len(src.locals)),
		stack:  make([]bool, len(src.stack)),
	}
	copy(dst.locals, src.locals)
	copy(dst.stack, src.stack)
	return dst
}

func mergeNonNilState(dst *nonNilState, src *nonNilState) bool {
	if dst == nil || src == nil {
		return false
	}

	changed := false
	if len(dst.locals) > len(src.locals) {
		for i := len(src.locals); i < len(dst.locals); i++ {
			if dst.locals[i] {
				dst.locals[i] = false
				changed = true
			}
		}
	}
	limit := len(dst.locals)
	if len(src.locals) < limit {
		limit = len(src.locals)
	}
	for i := 0; i < limit; i++ {
		merged := dst.locals[i] && src.locals[i]
		if dst.locals[i] != merged {
			dst.locals[i] = merged
			changed = true
		}
	}

	if len(dst.stack) != len(src.stack) {
		limit = len(dst.stack)
		if len(src.stack) < limit {
			limit = len(src.stack)
		}
		merged := make([]bool, limit)
		for i := 0; i < limit; i++ {
			merged[i] = dst.stack[i] && src.stack[i]
		}
		if len(dst.stack) != len(merged) {
			changed = true
		} else {
			for i := 0; i < len(merged); i++ {
				if dst.stack[i] != merged[i] {
					changed = true
					break
				}
			}
		}
		dst.stack = merged
		return changed
	}

	for i := 0; i < len(dst.stack); i++ {
		merged := dst.stack[i] && src.stack[i]
		if dst.stack[i] != merged {
			dst.stack[i] = merged
			changed = true
		}
	}
	return changed
}

func pushNonNil(state *nonNilState, v bool) {
	if state == nil {
		return
	}
	state.stack = append(state.stack, v)
}

func popNonNil(state *nonNilState) bool {
	if state == nil || len(state.stack) == 0 {
		return false
	}
	i := len(state.stack) - 1
	v := state.stack[i]
	state.stack = state.stack[:i]
	return v
}

func topNonNil(state *nonNilState) bool {
	if state == nil || len(state.stack) == 0 {
		return false
	}
	return state.stack[len(state.stack)-1]
}

func dropNonNil(state *nonNilState, count int) {
	if state == nil || count <= 0 {
		return
	}
	if count >= len(state.stack) {
		state.stack = state.stack[:0]
		return
	}
	state.stack = state.stack[:len(state.stack)-count]
}

func setLocalNonNil(state *nonNilState, idx int, v bool) {
	if state == nil || idx < 0 || idx >= len(state.locals) {
		return
	}
	state.locals[idx] = v
}

func getLocalNonNil(state *nonNilState, idx int) bool {
	if state == nil || idx < 0 || idx >= len(state.locals) {
		return false
	}
	return state.locals[idx]
}

func clearStackNonNil(state *nonNilState) {
	if state == nil {
		return
	}
	for i := range state.stack {
		state.stack[i] = false
	}
}

func isZeroConstInst(inst Inst) bool {
	switch inst.Op {
	case OP_CONST_NIL:
		return true
	case OP_CONST_BOOL:
		return inst.Arg == 0
	case OP_CONST_I64:
		return inst.Val == 0
	default:
		return false
	}
}

func callResultProvablyNonNil(inst Inst) bool {
	if inst.Op != OP_CALL {
		return false
	}
	if inst.Name == "runtime.Alloc" {
		return true
	}
	if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
		return true
	}
	return len(inst.Name) > 22 && inst.Name[0:22] == "runtime.$rtgComposite$"
}

func convertPreservesNonNil(name string) bool {
	switch name {
	case "int", "uintptr", "uint", "int64", "uint64":
		return true
	default:
		return false
	}
}

func instRetCount(inst Inst, f *IRFunc, funcRetCounts map[string]int, ifaceMethodRets map[string]int) int {
	switch inst.Op {
	case OP_CALL:
		if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
			return 1
		}
		if funcRetCounts != nil {
			if n, ok := funcRetCounts[inst.Name]; ok {
				return n
			}
		}
	case OP_CALL_INTRINSIC:
		if f != nil {
			return f.RetCount
		}
	case OP_IFACE_CALL:
		if ifaceMethodRets != nil && len(inst.Name) > 0 {
			dot := len(inst.Name) - 1
			for dot >= 0 && inst.Name[dot] != '.' {
				dot--
			}
			if dot > 0 && dot+1 < len(inst.Name) {
				if n, ok := ifaceMethodRets[inst.Name[:dot]+"\x00"+inst.Name[dot+1:]]; ok {
					return n
				}
			}
		}
	}
	return 0
}

func compareJumpProvenNonNilLocal(code []Inst, i int) (int, bool) {
	if i < 2 {
		return 0, false
	}
	a := code[i-2]
	b := code[i-1]
	if a.Op == OP_LOCAL_GET && isZeroConstInst(b) {
		return a.Arg, true
	}
	if b.Op == OP_LOCAL_GET && isZeroConstInst(a) {
		return b.Arg, true
	}
	if i >= 3 {
		a = code[i-3]
		b = code[i-2]
		c := code[i-1]
		if a.Op == OP_LOCAL_GET && (b.Op == OP_LEN || b.Op == OP_CAP) && isZeroConstInst(c) {
			return a.Arg, true
		}
		if c.Op == OP_LOCAL_GET && isZeroConstInst(a) && (b.Op == OP_LEN || b.Op == OP_CAP) {
			return c.Arg, true
		}
	}
	return 0, false
}

func branchProvenNonNilLocal(code []Inst, i int, onTarget bool) (int, bool) {
	if i < 0 || i >= len(code) {
		return 0, false
	}
	inst := code[i]
	switch inst.Op {
	case OP_JMP_IF:
		if onTarget && i > 0 && code[i-1].Op == OP_LOCAL_GET {
			return code[i-1].Arg, true
		}
		if onTarget && i > 1 && (code[i-1].Op == OP_LEN || code[i-1].Op == OP_CAP) && code[i-2].Op == OP_LOCAL_GET {
			return code[i-2].Arg, true
		}
	case OP_JMP_IF_NOT:
		if !onTarget && i > 0 && code[i-1].Op == OP_LOCAL_GET {
			return code[i-1].Arg, true
		}
		if !onTarget && i > 1 && (code[i-1].Op == OP_LEN || code[i-1].Op == OP_CAP) && code[i-2].Op == OP_LOCAL_GET {
			return code[i-2].Arg, true
		}
	case OP_JMP_NEQ:
		if onTarget {
			return compareJumpProvenNonNilLocal(code, i)
		}
	case OP_JMP_EQ:
		if !onTarget {
			return compareJumpProvenNonNilLocal(code, i)
		}
	}
	return 0, false
}

func transferNonNilState(state *nonNilState, inst Inst, f *IRFunc, funcRetCounts map[string]int, ifaceMethodRets map[string]int) *nonNilState {
	next := cloneNonNilState(state)
	if next == nil {
		return nil
	}

	switch inst.Op {
	case OP_CONST_I64:
		pushNonNil(next, inst.Val != 0)
	case OP_CONST_STR:
		pushNonNil(next, len(inst.Name) > 0)
	case OP_CONST_BOOL:
		pushNonNil(next, inst.Arg != 0)
	case OP_CONST_NIL:
		pushNonNil(next, false)
	case OP_LOCAL_GET:
		pushNonNil(next, getLocalNonNil(next, inst.Arg))
	case OP_LOCAL_SET:
		setLocalNonNil(next, inst.Arg, popNonNil(next))
	case OP_LOCAL_ADD_IMM:
		if inst.Val != 0 {
			setLocalNonNil(next, inst.Arg, false)
		}
	case OP_LOCAL_ADDR:
		pushNonNil(next, true)
	case OP_GLOBAL_GET:
		pushNonNil(next, false)
	case OP_GLOBAL_SET:
		popNonNil(next)
	case OP_GLOBAL_ADDR:
		pushNonNil(next, true)
	case OP_DROP:
		popNonNil(next)
	case OP_DUP:
		pushNonNil(next, topNonNil(next))
	case OP_ADD, OP_SUB, OP_MUL, OP_DIV, OP_MOD, OP_AND, OP_OR, OP_XOR, OP_SHL, OP_SHR:
		popNonNil(next)
		popNonNil(next)
		pushNonNil(next, false)
	case OP_EQ, OP_NEQ, OP_LT, OP_GT, OP_LEQ, OP_GEQ:
		popNonNil(next)
		popNonNil(next)
		pushNonNil(next, false)
	case OP_JMP_EQ, OP_JMP_NEQ, OP_JMP_LT, OP_JMP_GT, OP_JMP_LEQ, OP_JMP_GEQ:
		popNonNil(next)
		popNonNil(next)
	case OP_NEG:
		v := popNonNil(next)
		pushNonNil(next, v)
	case OP_NOT:
		popNonNil(next)
		pushNonNil(next, false)
	case OP_LOAD, OP_LEN, OP_CAP:
		popNonNil(next)
		pushNonNil(next, false)
	case OP_STORE:
		popNonNil(next)
		popNonNil(next)
	case OP_OFFSET:
		v := popNonNil(next)
		pushNonNil(next, v && inst.Arg >= 0)
	case OP_LABEL, OP_JMP:
		// No stack effect.
	case OP_JMP_IF, OP_JMP_IF_NOT:
		popNonNil(next)
	case OP_CALL:
		dropNonNil(next, inst.Arg)
		retCount := instRetCount(inst, f, funcRetCounts, ifaceMethodRets)
		for i := 0; i < retCount; i++ {
			pushNonNil(next, false)
		}
		if retCount == 1 && callResultProvablyNonNil(inst) {
			next.stack[len(next.stack)-1] = true
		}
	case OP_CALL_INTRINSIC:
		retCount := instRetCount(inst, f, funcRetCounts, ifaceMethodRets)
		for i := 0; i < retCount; i++ {
			pushNonNil(next, false)
		}
	case OP_RETURN:
		dropNonNil(next, inst.Arg)
	case OP_SLICE_GET, OP_SLICE_MAKE, OP_STRING_GET, OP_STRING_MAKE:
		clearStackNonNil(next)
	case OP_INDEX_ADDR:
		popNonNil(next)
		popNonNil(next)
		pushNonNil(next, false)
	case OP_CONVERT:
		v := popNonNil(next)
		pushNonNil(next, convertPreservesNonNil(inst.Name) && v)
	case OP_IFACE_BOX:
		popNonNil(next)
		pushNonNil(next, true)
	case OP_IFACE_CALL:
		dropNonNil(next, inst.Arg+1)
		retCount := instRetCount(inst, f, funcRetCounts, ifaceMethodRets)
		for i := 0; i < retCount; i++ {
			pushNonNil(next, false)
		}
	case OP_PANIC:
		popNonNil(next)
	}

	return next
}

// annotateNonNilMemoryBases marks selected LOAD instructions with a backend
// hint when their input address is provably non-zero.
func annotateNonNilMemoryBases(code []Inst, f *IRFunc, funcRetCounts map[string]int, ifaceMethodRets map[string]int) ([]Inst, bool) {
	if len(code) == 0 {
		return code, false
	}

	numLocals := 0
	if f != nil {
		numLocals = len(f.Locals)
	}
	changed := false
	out := make([]Inst, len(code))
	copy(out, code)
	labels := buildLabelIndex(code)
	in := make([]*nonNilState, len(code))
	in[0] = &nonNilState{locals: make([]bool, numLocals)}
	work := []int{0}

	enqueue := func(idx int, state *nonNilState) {
		if idx < 0 || idx >= len(code) || state == nil {
			return
		}
		if in[idx] == nil {
			in[idx] = cloneNonNilState(state)
			work = append(work, idx)
			return
		}
		if mergeNonNilState(in[idx], state) {
			work = append(work, idx)
		}
	}

	for len(work) > 0 {
		i := work[len(work)-1]
		work = work[:len(work)-1]
		state := cloneNonNilState(in[i])
		if state == nil {
			continue
		}

		cur := out[i]
		if cur.Op != OP_LOAD {
			// Nothing to mark.
		} else if cur.Name == "" && topNonNil(state) {
			out[i].Name = InstNonNilMemoryBase
			changed = true
		}

		next := transferNonNilState(state, code[i], f, funcRetCounts, ifaceMethodRets)
		switch code[i].Op {
		case OP_JMP:
			if target, ok := labels[code[i].Arg]; ok {
				enqueue(target, next)
			}
		case OP_JMP_IF, OP_JMP_IF_NOT, OP_JMP_EQ, OP_JMP_NEQ, OP_JMP_LT, OP_JMP_GT, OP_JMP_LEQ, OP_JMP_GEQ:
			if target, ok := labels[code[i].Arg]; ok {
				targetState := cloneNonNilState(next)
				if idx, ok := branchProvenNonNilLocal(code, i, true); ok {
					setLocalNonNil(targetState, idx, true)
				}
				enqueue(target, targetState)
			}
			if i+1 < len(code) {
				fallthroughState := cloneNonNilState(next)
				if idx, ok := branchProvenNonNilLocal(code, i, false); ok {
					setLocalNonNil(fallthroughState, idx, true)
				}
				enqueue(i+1, fallthroughState)
			}
		case OP_RETURN, OP_PANIC:
			// No successors.
		default:
			if i+1 < len(code) {
				enqueue(i+1, next)
			}
		}
	}

	if !changed {
		return code, false
	}
	return out, true
}

// deadLocalStoreToDrop rewrites LOCAL_SET to DROP when the local is never read
// and its address is never taken anywhere in the function.
func deadLocalStoreToDrop(code []Inst, numLocals int) ([]Inst, bool) {
	if len(code) == 0 || numLocals <= 0 {
		return code, false
	}

	localRead := make([]bool, numLocals)
	localAddrTaken := make([]bool, numLocals)

	for _, inst := range code {
		switch inst.Op {
		case OP_LOCAL_GET:
			if inst.Arg >= 0 && inst.Arg < numLocals {
				localRead[inst.Arg] = true
			}
		case OP_LOCAL_ADDR:
			if inst.Arg >= 0 && inst.Arg < numLocals {
				localAddrTaken[inst.Arg] = true
			}
		}
	}

	changed := false
	out := make([]Inst, len(code))
	copy(out, code)
	for i := range out {
		inst := out[i]
		if inst.Op != OP_LOCAL_SET {
			continue
		}
		if inst.Arg < 0 || inst.Arg >= numLocals {
			continue
		}
		if localRead[inst.Arg] || localAddrTaken[inst.Arg] {
			continue
		}

		out[i] = makeInst(OP_DROP, 0, 0, 0, "")
		changed = true
	}
	return out, changed
}

func buildLabelIndex(code []Inst) map[int]int {
	labels := make(map[int]int)
	for i, inst := range code {
		if inst.Op == OP_LABEL {
			if _, exists := labels[inst.Arg]; !exists {
				labels[inst.Arg] = i
			}
		}
	}
	return labels
}

func removeUnreachableIRCode(code []Inst) ([]Inst, bool) {
	if len(code) == 0 {
		return code, false
	}

	labels := buildLabelIndex(code)
	reachable := make([]bool, len(code))
	work := []int{0}
	reachable[0] = true

	for len(work) > 0 {
		i := work[len(work)-1]
		work = work[:len(work)-1]

		inst := code[i]

		switch inst.Op {
		case OP_JMP:
			if target, ok := labels[inst.Arg]; ok {
				if target >= 0 && target < len(code) && !reachable[target] {
					reachable[target] = true
					work = append(work, target)
				}
			}
		case OP_JMP_IF, OP_JMP_IF_NOT, OP_JMP_EQ, OP_JMP_NEQ, OP_JMP_LT, OP_JMP_GT, OP_JMP_LEQ, OP_JMP_GEQ:
			next := i + 1
			if next >= 0 && next < len(code) && !reachable[next] {
				reachable[next] = true
				work = append(work, next)
			}
			if target, ok := labels[inst.Arg]; ok {
				if target >= 0 && target < len(code) && !reachable[target] {
					reachable[target] = true
					work = append(work, target)
				}
			}
		case OP_RETURN, OP_PANIC:
			// Terminators: no fallthrough.
		default:
			next := i + 1
			if next >= 0 && next < len(code) && !reachable[next] {
				reachable[next] = true
				work = append(work, next)
			}
		}
	}

	changed := false
	filtered := make([]Inst, 0, len(code))
	for i, inst := range code {
		if reachable[i] {
			filtered = append(filtered, inst)
			continue
		}
		changed = true
	}
	return filtered, changed
}

// mergeReturnsToSharedEpilogue rewrites functions with multiple RETURN sites
// to branch to one shared RETURN block.
func mergeReturnsToSharedEpilogue(code []Inst, retCount int) ([]Inst, bool) {
	if len(code) < 2 {
		return code, false
	}
	// For non-void functions, RETURN carries result stack semantics in some
	// backends (notably wasm), so merging via JMP is not always valid.
	if retCount != 0 {
		return code, false
	}

	returnCount := 0
	lastReturnIdx := -1
	for i, inst := range code {
		if inst.Op == OP_RETURN {
			returnCount++
			lastReturnIdx = i
		}
	}
	if returnCount <= 1 || lastReturnIdx < 0 {
		return code, false
	}

	epLabel := nextFreshLabelID(code)
	out := make([]Inst, 0, len(code)+1)
	changed := false

	for i, inst := range code {
		if inst.Op != OP_RETURN {
			out = append(out, inst)
			continue
		}
		if i == lastReturnIdx {
			out = append(out, makeInst(OP_LABEL, epLabel, 0, 0, ""))
			out = append(out, inst)
			continue
		}
		out = append(out, makeInst(OP_JMP, epLabel, 0, 0, ""))
		changed = true
	}

	return out, changed
}

func nextFreshLabelID(code []Inst) int {
	maxLabel := 0
	found := false
	for _, inst := range code {
		if inst.Op != OP_LABEL {
			continue
		}
		if !found || inst.Arg > maxLabel {
			maxLabel = inst.Arg
			found = true
		}
	}
	if !found {
		return 1
	}
	return maxLabel + 1
}

func removeRedundantFallthroughJumps(code []Inst) ([]Inst, bool) {
	changed := false
	out := make([]Inst, 0, len(code))
	for i := 0; i < len(code); i++ {
		inst := code[i]
		if inst.Op == OP_JMP {
			remove := false
			j := i + 1
			for j < len(code) && code[j].Op == OP_LABEL {
				if code[j].Arg == inst.Arg {
					changed = true
					remove = true
					break
				}
				j++
			}
			if remove {
				continue
			}
		}
		out = append(out, inst)
	}
	return out, changed
}

func threadJumps(code []Inst) ([]Inst, bool) {
	labels := buildLabelIndex(code)
	changed := false

	out := make([]Inst, len(code))
	copy(out, code)

	for i := range out {
		if out[i].Op != OP_JMP {
			continue
		}
		if isShortCircuitGuardJump(out, i) {
			continue
		}

		target := out[i].Arg
		visited := make(map[int]bool)
		for {
			if visited[target] {
				break
			}
			visited[target] = true

			targetIdx, ok := labels[target]
			if !ok {
				break
			}

			j := targetIdx + 1
			for j < len(out) && out[j].Op == OP_LABEL {
				j++
			}
			if j >= len(out) || out[j].Op != OP_JMP {
				break
			}
			if out[j].Arg == target {
				break
			}
			target = out[j].Arg
		}

		if target != out[i].Arg {
			out[i].Arg = target
			changed = true
		}
	}

	return out, changed
}

// isShortCircuitGuardJump reports whether code[pos] is the "JMP endLabel"
// guard in the canonical short-circuit pattern:
//
//	... JMP endLabel, LABEL targetLabel, CONST, LABEL endLabel
//
// The WASM backend pattern matcher depends on this exact adjacency.
func isShortCircuitGuardJump(code []Inst, pos int) bool {
	if pos < 0 || pos+3 >= len(code) {
		return false
	}
	if code[pos].Op != OP_JMP {
		return false
	}
	if code[pos+1].Op != OP_LABEL {
		return false
	}
	if code[pos+2].Op != OP_CONST_BOOL && code[pos+2].Op != OP_CONST_I64 {
		return false
	}
	if code[pos+3].Op != OP_LABEL {
		return false
	}
	return code[pos].Arg == code[pos+3].Arg
}

func removeUnreferencedLabels(code []Inst) ([]Inst, bool) {
	referenced := make(map[int]bool)
	for _, inst := range code {
		switch inst.Op {
		case OP_JMP, OP_JMP_IF, OP_JMP_IF_NOT, OP_JMP_EQ, OP_JMP_NEQ, OP_JMP_LT, OP_JMP_GT, OP_JMP_LEQ, OP_JMP_GEQ:
			referenced[inst.Arg] = true
		}
	}

	changed := false
	out := make([]Inst, 0, len(code))
	for _, inst := range code {
		if inst.Op == OP_LABEL && !referenced[inst.Arg] {
			changed = true
			continue
		}
		out = append(out, inst)
	}
	return out, changed
}
