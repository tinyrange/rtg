package main

// optimizeIRModule runs lightweight, backend-independent IR cleanups.
func optimizeIRModule(irmod *IRModule) {
	for _, f := range irmod.Funcs {
		f.Code = optimizeIRFuncCode(f.Code)
	}
}

func optimizeIRFuncCode(code []Inst) []Inst {
	if len(code) == 0 {
		return code
	}

	changed := true
	for changed {
		changed = false

		var stepChanged bool
		code, stepChanged = removeUnreachableIRCode(code)
		if stepChanged {
			changed = true
		}

		code, stepChanged = removeRedundantFallthroughJumps(code)
		if stepChanged {
			changed = true
		}

		code, stepChanged = threadJumps(code)
		if stepChanged {
			changed = true
		}

		code, stepChanged = removeUnreferencedLabels(code)
		if stepChanged {
			changed = true
		}
	}

	return code
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
		case OP_JMP_IF, OP_JMP_IF_NOT:
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

func removeUnreferencedLabels(code []Inst) ([]Inst, bool) {
	referenced := make(map[int]bool)
	for _, inst := range code {
		switch inst.Op {
		case OP_JMP, OP_JMP_IF, OP_JMP_IF_NOT:
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
