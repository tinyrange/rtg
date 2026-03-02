package ir

import "j5.nz/rtg/std/compiler/common"

// OptimizeIRModule runs lightweight, backend-independent IR cleanups.
func OptimizeIRModule(target *common.Target, irmod *IRModule) []string {
	var errors []string
	errors = append(errors, inlineZeroCallFuncs(irmod)...)
	for _, f := range irmod.Funcs {
		f.Code = optimizeIRFuncCode(target, f)
	}
	return errors
}

func optimizeIRFuncCode(target *common.Target, f *IRFunc) []Inst {
	code := f.Code
	if len(code) == 0 {
		return code
	}

	changed := true
	for changed {
		changed = false

		var stepChanged bool
		code, stepChanged = foldLocalAddImm(code, f.Locals)
		if stepChanged {
			changed = true
		}

		code, stepChanged = foldNotConditionalJumps(code)
		if stepChanged {
			changed = true
		}

		code, stepChanged = foldSliceAppendU32LE(code)
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
					imm32 := int32(imm)
					if int64(imm32) == imm {
						out = append(out, Inst{
							Op:  OP_LOCAL_ADD_IMM,
							Arg: idx,
							Val: int64(imm32),
						})
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
		if i+45 < len(code) && matchesSliceAppendU64LEWindow(code, i) {
			v := code[i]
			if !changed {
				out = make([]Inst, 0, len(code))
				out = append(out, code[:i]...)
			}
			out = append(out, v)
			out = append(out, Inst{
				Op:   OP_CALL,
				Name: "runtime.SliceAppendU64LE",
				Arg:  2,
			})
			changed = true
			i += 46
			continue
		}
		if i+21 < len(code) && matchesSliceAppendU32LEWindow(code, i) {
			v := code[i]
			if !changed {
				out = make([]Inst, 0, len(code))
				out = append(out, code[:i]...)
			}
			out = append(out, v)
			out = append(out, Inst{
				Op:   OP_CALL,
				Name: "runtime.SliceAppendU32LE",
				Arg:  2,
			})
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

		out[i] = Inst{Op: OP_DROP}
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
