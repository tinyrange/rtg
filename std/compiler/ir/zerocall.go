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

func nextIRLabelID(code []Inst) int {
	maxLabel := 0
	seen := false
	for _, inst := range code {
		if !zeroCallIsLabelOp(inst.Op) {
			continue
		}
		if !seen || inst.Arg > maxLabel {
			maxLabel = inst.Arg
			seen = true
		}
	}
	if !seen {
		return 0
	}
	return maxLabel + 1
}

func appendUniqueString(list []string, value string) []string {
	for i := 0; i < len(list); i++ {
		if list[i] == value {
			return list
		}
	}
	return append(list, value)
}

func detectZeroCallCyclesVisit(
	name string,
	edges map[string][]string,
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
	if nexts := edges[name]; len(nexts) > 0 {
		nextNames := append([]string{}, nexts...)
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

func detectZeroCallCycles(edges map[string][]string, zeroCall map[string]bool) []string {
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

func validateZeroCallFuncs(irmod *IRModule, funcIndex map[string]*IRFunc) (map[string][]string, []string) {
	edges := make(map[string][]string)
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
		for _, inst := range f.Code {
			switch inst.Op {
			case OP_CALL_INTRINSIC:
				errs = append(errs, fmt.Sprintf("zerocall function %s uses intrinsic call %s (unsupported)", name, inst.Name))
			case OP_IFACE_CALL:
				errs = append(errs, fmt.Sprintf("zerocall function %s uses interface dispatch %s (unsupported)", name, inst.Name))
			case OP_CALL:
				if strings.HasPrefix(inst.Name, "builtin.composite.") {
					errs = append(errs, fmt.Sprintf("zerocall function %s uses composite helper call %s (unsupported)", name, inst.Name))
					continue
				}
				if !irmod.ZeroCallFuncs[inst.Name] {
					errs = append(errs, fmt.Sprintf("zerocall function %s calls non-zerocall function %s", name, inst.Name))
					continue
				}
				edges[name] = appendUniqueString(edges[name], inst.Name)
			}
		}
	}
	if len(errs) > 0 {
		return edges, errs
	}
	errs = append(errs, detectZeroCallCycles(edges, irmod.ZeroCallFuncs)...)
	return edges, errs
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

	for _, inst := range caller.Code {
		if inst.Op != OP_CALL || !zeroCall[inst.Name] {
			out = append(out, inst)
			continue
		}
		callee, ok := funcIndex[inst.Name]
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: zerocall target missing: %s", caller.Name, inst.Name))
			out = append(out, inst)
			continue
		}
		if callee.Native != nil {
			errs = append(errs, fmt.Sprintf("%s: zerocall target is native and cannot be inlined: %s", caller.Name, inst.Name))
			out = append(out, inst)
			continue
		}

		base := len(caller.Locals) + len(newLocals)
		frameSlots := len(callee.Locals)
		if callee.Params > frameSlots {
			frameSlots = callee.Params
		}
		for i := 0; i < frameSlots; i++ {
			local := IRLocal{
				Name:  "$zerocall",
				Index: base + i,
			}
			if i < len(callee.Locals) {
				local.Type = callee.Locals[i].Type
				local.Is64 = callee.Locals[i].Is64
				local.Width = callee.Locals[i].Width
			}
			newLocals = append(newLocals, local)
		}

		for p := callee.Params - 1; p >= 0; p-- {
			out = append(out, Inst{Op: OP_LOCAL_SET, Arg: base + p})
		}

		labelMap := make(map[int]int)
		for _, cinst := range callee.Code {
			if zeroCallIsLabelOp(cinst.Op) {
				if _, exists := labelMap[cinst.Arg]; !exists {
					labelMap[cinst.Arg] = nextLabel
					nextLabel++
				}
			}
		}
		exitLabel := nextLabel
		nextLabel++

		for _, cinst := range callee.Code {
			if cinst.Op == OP_RETURN {
				out = append(out, Inst{Op: OP_JMP, Arg: exitLabel})
				continue
			}
			cloned := cinst
			if zeroCallIsLocalOp(cloned.Op) {
				cloned.Arg = base + cloned.Arg
			}
			if zeroCallIsLabelOp(cloned.Op) {
				mapped, ok := labelMap[cloned.Arg]
				if !ok {
					mapped = nextLabel
					nextLabel++
					labelMap[cloned.Arg] = mapped
				}
				cloned.Arg = mapped
			}
			out = append(out, cloned)
		}
		out = append(out, Inst{Op: OP_LABEL, Arg: exitLabel})
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
		for _, inst := range f.Code {
			if inst.Op == OP_CALL && irmod.ZeroCallFuncs[inst.Name] {
				errs = append(errs, fmt.Sprintf("%s: zerocall call was not inlined: %s", f.Name, inst.Name))
			}
		}
	}
	return errs
}
