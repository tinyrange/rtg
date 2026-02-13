package main

// intrinsicRuntimeDep returns the runtime function name that an intrinsic
// depends on, or "" if none.
func intrinsicRuntimeDep(name string) string {
	if name == "Tostring" {
		return "runtime.IntToString"
	}
	return ""
}

// dceAddRoot adds a function name to the reachable set and worklist if it
// exists in funcIndex and hasn't been visited yet.
func dceAddRoot(name string, funcIndex map[string]int, reachable map[string]bool, worklist []string) []string {
	_, exists := funcIndex[name]
	if exists {
		if !reachable[name] {
			reachable[name] = true
			worklist = append(worklist, name)
		}
	}
	return worklist
}

// eliminateDeadFunctions removes unreachable functions from the IR module
// using a mark-and-sweep reachability analysis starting from main.main,
// init functions, interface method implementations, and backend-implicit roots.
func eliminateDeadFunctions(irmod *IRModule) {
	// Build name→index for fast lookup
	funcIndex := make(map[string]int)
	for i, f := range irmod.Funcs {
		funcIndex[f.Name] = i
	}

	// Reachable set and worklist
	reachable := make(map[string]bool)
	var worklist []string

	// Root set: main.main
	worklist = dceAddRoot("main.main", funcIndex, reachable, worklist)

	// Root set: init functions
	for _, f := range irmod.Funcs {
		if isInitFunc(f.Name) {
			worklist = dceAddRoot(f.Name, funcIndex, reachable, worklist)
		}
	}

	// Root set: interface method implementations (all values in MethodTable)
	for _, funcName := range irmod.MethodTable {
		worklist = dceAddRoot(funcName, funcIndex, reachable, worklist)
	}

	// Root set: backend-implicit runtime functions
	worklist = dceAddRoot("runtime.Alloc", funcIndex, reachable, worklist)
	worklist = dceAddRoot("runtime.Makestring", funcIndex, reachable, worklist)
	worklist = dceAddRoot("runtime.runtimePanic", funcIndex, reachable, worklist)

	// BFS: scan each reachable function for call edges
	for len(worklist) > 0 {
		name := worklist[len(worklist)-1]
		worklist = worklist[0 : len(worklist)-1]

		idx, ok := funcIndex[name]
		if !ok {
			continue
		}
		f := irmod.Funcs[idx]

		for _, inst := range f.Code {
			if inst.Op == OP_CALL {
				// Skip synthetic composite literal calls
				if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
					continue
				}
				if !reachable[inst.Name] {
					reachable[inst.Name] = true
					worklist = append(worklist, inst.Name)
				}
			} else if inst.Op == OP_CALL_INTRINSIC {
				dep := intrinsicRuntimeDep(inst.Name)
				if dep != "" {
					if !reachable[dep] {
						reachable[dep] = true
						worklist = append(worklist, dep)
					}
				}
			} else if inst.Op == OP_CONVERT {
				// Backends emit runtime calls for certain type conversions
				if inst.Name == "string" {
					if !reachable["runtime.BytesToString"] {
						reachable["runtime.BytesToString"] = true
						worklist = append(worklist, "runtime.BytesToString")
					}
				} else if inst.Name == "[]byte" {
					if !reachable["runtime.StringToBytes"] {
						reachable["runtime.StringToBytes"] = true
						worklist = append(worklist, "runtime.StringToBytes")
					}
				}
			}
		}
	}

	// Sweep: filter Funcs to keep only reachable ones, preserving order
	filtered := make([]*IRFunc, 0, len(reachable))
	for _, f := range irmod.Funcs {
		if reachable[f.Name] {
			filtered = append(filtered, f)
		}
	}
	irmod.Funcs = filtered
}
