package ir

import (
	"fmt"
	"sort"
	"strings"
)

func sortedStringKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func zeroCallIsLocalOp(op Opcode) bool {
	return op == OP_LOCAL_GET || op == OP_LOCAL_SET || op == OP_LOCAL_ADD_IMM || op == OP_LOCAL_ADDR
}

func zeroCallIsLabelOp(op Opcode) bool {
	return op == OP_LABEL ||
		op == OP_JMP ||
		op == OP_JMP_IF ||
		op == OP_JMP_IF_NOT ||
		op == OP_JMP_EQ ||
		op == OP_JMP_NEQ ||
		op == OP_JMP_LT ||
		op == OP_JMP_GT ||
		op == OP_JMP_LEQ ||
		op == OP_JMP_GEQ
}

func zeroCallIsCompositeHelper(name string) bool {
	return len(name) > 18 && name[0:18] == "builtin.composite."
}

func nextIRLabelID(code []Inst) int {
	maxLabel := 0
	seen := false
	for i := 0; i < len(code); i++ {
		if !zeroCallIsLabelOp(code[i].Op) {
			continue
		}
		if !seen || code[i].Arg > maxLabel {
			maxLabel = code[i].Arg
			seen = true
		}
	}
	if !seen {
		return 0
	}
	return maxLabel + 1
}

func detectZeroCallCyclesVisit(
	name string,
	edges map[string]map[string]bool,
	zeroCall map[string]bool,
	state map[string]int,
	stack *[]string,
	stackPos map[string]int,
	errs *[]string,
) {
	if len(*errs) > 0 {
		return
	}
	state[name] = 1
	stackPos[name] = len(*stack)
	*stack = append(*stack, name)
	if nexts := edges[name]; nexts != nil {
		nextNames := make([]string, 0, len(nexts))
		for next := range nexts {
			nextNames = append(nextNames, next)
		}
		sort.Strings(nextNames)
		for _, next := range nextNames {
			if !zeroCall[next] {
				continue
			}
			if state[next] == 0 {
				detectZeroCallCyclesVisit(next, edges, zeroCall, state, stack, stackPos, errs)
				if len(*errs) > 0 {
					return
				}
				continue
			}
			if state[next] == 1 {
				start := stackPos[next]
				cycle := append([]string{}, (*stack)[start:len(*stack)]...)
				cycle = append(cycle, next)
				*errs = append(*errs, fmt.Sprintf("zerocall cycle detected: %s", strings.Join(cycle, " -> ")))
				return
			}
		}
	}
	*stack = (*stack)[0 : len(*stack)-1]
	delete(stackPos, name)
	state[name] = 2
}

func detectZeroCallCycles(edges map[string]map[string]bool, zeroCall map[string]bool) []string {
	state := make(map[string]int) // 0=unvisited, 1=visiting, 2=done
	stack := []string{}
	stackPos := make(map[string]int)
	var errs []string
	for _, name := range sortedStringKeys(zeroCall) {
		if state[name] == 0 {
			detectZeroCallCyclesVisit(name, edges, zeroCall, state, &stack, stackPos, &errs)
			if len(errs) > 0 {
				return errs
			}
		}
	}
	return nil
}

func validateZeroCallFuncs(irmod *IRModule, funcIndex map[string]*IRFunc) (map[string]map[string]bool, []string) {
	edges := make(map[string]map[string]bool)
	var errs []string

	for _, name := range sortedStringKeys(irmod.ZeroCallFuncs) {
		f, ok := funcIndex[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("zerocall function not found in IR: %s", name))
			continue
		}
		if f.Native != nil {
			errs = append(errs, fmt.Sprintf("zerocall function %s is native and cannot be inlined", name))
			continue
		}
		for i := 0; i < len(f.Code); i++ {
			switch f.Code[i].Op {
			case OP_CALL_INTRINSIC:
				errs = append(errs, fmt.Sprintf("zerocall function %s uses intrinsic call %s (unsupported)", name, f.Code[i].Name))
			case OP_IFACE_CALL:
				errs = append(errs, fmt.Sprintf("zerocall function %s uses interface dispatch %s (unsupported)", name, f.Code[i].Name))
			case OP_CALL:
				if zeroCallIsCompositeHelper(f.Code[i].Name) {
					errs = append(errs, fmt.Sprintf("zerocall function %s uses composite helper call %s (unsupported)", name, f.Code[i].Name))
					continue
				}
				if !irmod.ZeroCallFuncs[f.Code[i].Name] {
					errs = append(errs, fmt.Sprintf("zerocall function %s calls non-zerocall function %s", name, f.Code[i].Name))
					continue
				}
				if edges[name] == nil {
					edges[name] = make(map[string]bool)
				}
				edges[name][f.Code[i].Name] = true
			}
		}
	}
	if len(errs) > 0 {
		return edges, errs
	}
	errs = append(errs, detectZeroCallCycles(edges, irmod.ZeroCallFuncs)...)
	return edges, errs
}

func appendZeroCallInst(out []Inst, op Opcode, arg int, width int, val int64, name string) []Inst {
	out = append(out, Inst{})
	idx := len(out) - 1
	out[idx].Op = op
	out[idx].Arg = arg
	out[idx].Width = width
	out[idx].Val = val
	out[idx].Name = name
	return out
}

func inlineZeroCallsInFunc(caller *IRFunc, funcIndex map[string]*IRFunc, zeroCall map[string]bool) ([]Inst, []IRLocal, bool, []string) {
	if caller == nil || len(caller.Code) == 0 {
		return nil, nil, false, nil
	}
	nextLabel := nextIRLabelID(caller.Code)
	out := make([]Inst, 0, len(caller.Code))
	var newLocals []IRLocal
	changed := false
	callSite := 0
	var errs []string

	for callIdx := 0; callIdx < len(caller.Code); callIdx++ {
		if caller.Code[callIdx].Op != OP_CALL || !zeroCall[caller.Code[callIdx].Name] {
			out = appendZeroCallInst(out, caller.Code[callIdx].Op, caller.Code[callIdx].Arg, caller.Code[callIdx].Width, caller.Code[callIdx].Val, caller.Code[callIdx].Name)
			continue
		}
		callee, ok := funcIndex[caller.Code[callIdx].Name]
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: zerocall target missing: %s", caller.Name, caller.Code[callIdx].Name))
			out = appendZeroCallInst(out, caller.Code[callIdx].Op, caller.Code[callIdx].Arg, caller.Code[callIdx].Width, caller.Code[callIdx].Val, caller.Code[callIdx].Name)
			continue
		}
		if callee.Native != nil {
			errs = append(errs, fmt.Sprintf("%s: zerocall target is native and cannot be inlined: %s", caller.Name, caller.Code[callIdx].Name))
			out = appendZeroCallInst(out, caller.Code[callIdx].Op, caller.Code[callIdx].Arg, caller.Code[callIdx].Width, caller.Code[callIdx].Val, caller.Code[callIdx].Name)
			continue
		}

		base := len(caller.Locals) + len(newLocals)
		frameSlots := len(callee.Locals)
		if callee.Params > frameSlots {
			frameSlots = callee.Params
		}
		for localIdx := 0; localIdx < frameSlots; localIdx++ {
			local := IRLocal{
				Name:  fmt.Sprintf("$zerocall.%s.%d.%d", caller.Code[callIdx].Name, callSite, localIdx),
				Index: base + localIdx,
			}
			if localIdx < len(callee.Locals) {
				local.Type = callee.Locals[localIdx].Type
				local.Is64 = callee.Locals[localIdx].Is64
				local.Width = callee.Locals[localIdx].Width
			}
			newLocals = append(newLocals, local)
		}

		for p := callee.Params - 1; p >= 0; p-- {
			out = appendZeroCallInst(out, OP_LOCAL_SET, base+p, 0, 0, "")
		}

		labelMap := make(map[int]int)
		for j := 0; j < len(callee.Code); j++ {
			if zeroCallIsLabelOp(callee.Code[j].Op) {
				if _, exists := labelMap[callee.Code[j].Arg]; !exists {
					labelMap[callee.Code[j].Arg] = nextLabel
					nextLabel++
				}
			}
		}
		exitLabel := nextLabel
		nextLabel++

		for j := 0; j < len(callee.Code); j++ {
			if callee.Code[j].Op == OP_RETURN {
				out = appendZeroCallInst(out, OP_JMP, exitLabel, 0, 0, "")
				continue
			}
			op := callee.Code[j].Op
			arg := callee.Code[j].Arg
			width := callee.Code[j].Width
			val := callee.Code[j].Val
			name := callee.Code[j].Name
			if zeroCallIsLocalOp(op) {
				arg = base + arg
			}
			if zeroCallIsLabelOp(op) {
				mapped, ok := labelMap[arg]
				if !ok {
					mapped = nextLabel
					nextLabel++
					labelMap[arg] = mapped
				}
				arg = mapped
			}
			out = appendZeroCallInst(out, op, arg, width, val, name)
		}
		out = appendZeroCallInst(out, OP_LABEL, exitLabel, 0, 0, "")
		changed = true
		callSite++
	}
	return out, newLocals, changed, errs
}

func inlineZeroCallFuncs(irmod *IRModule) []string {
	if irmod == nil || len(irmod.ZeroCallFuncs) == 0 {
		return nil
	}
	funcIndex := make(map[string]*IRFunc)
	for _, f := range irmod.Funcs {
		funcIndex[f.Name] = f
	}
	_, errs := validateZeroCallFuncs(irmod, funcIndex)
	if len(errs) > 0 {
		return errs
	}

	changed := true
	for changed {
		changed = false
		for _, caller := range irmod.Funcs {
			newCode, newLocals, didChange, inlineErrs := inlineZeroCallsInFunc(caller, funcIndex, irmod.ZeroCallFuncs)
			if len(inlineErrs) > 0 {
				errs = append(errs, inlineErrs...)
				continue
			}
			if didChange {
				caller.Code = newCode
				caller.Locals = append(caller.Locals, newLocals...)
				changed = true
			}
		}
		if len(errs) > 0 {
			return errs
		}
	}

	for _, f := range irmod.Funcs {
		for i := 0; i < len(f.Code); i++ {
			if f.Code[i].Op == OP_CALL && irmod.ZeroCallFuncs[f.Code[i].Name] {
				errs = append(errs, fmt.Sprintf("%s: zerocall call was not inlined: %s", f.Name, f.Code[i].Name))
			}
		}
	}
	return errs
}
