package ir

// IsInitFunc checks if a function name is a package init function.
func IsInitFunc(name string) bool {
	n := len(name)
	if n < 5 {
		return false
	}
	// Match both ".init" and ".init$globals"
	if n >= 13 && name[n-13:n] == ".init$globals" {
		return true
	}
	return name[n-5:n] == ".init"
}

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

func dceMethodName(name string) string {
	lastDot := -1
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			lastDot = i
		}
	}
	if lastDot < 0 {
		return name
	}
	if lastDot+1 >= len(name) {
		return ""
	}
	return name[lastDot+1:]
}

// eliminateDeadFunctions removes unreachable functions from the IR module
// using a mark-and-sweep reachability analysis starting from main.main,
// init functions, and backend-implicit roots.
func EliminateDeadFunctions(irmod *IRModule) {
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
		if IsInitFunc(f.Name) {
			worklist = dceAddRoot(f.Name, funcIndex, reachable, worklist)
		}
	}

	// Root set: backend-implicit runtime functions
	worklist = dceAddRoot("runtime.Alloc", funcIndex, reachable, worklist)
	worklist = dceAddRoot("runtime.Makestring", funcIndex, reachable, worklist)
	worklist = dceAddRoot("runtime.runtimePanic", funcIndex, reachable, worklist)

	// Root set: callback functions (called by the OS, not by RTG code)
	for name := range irmod.CallbackFuncs {
		worklist = dceAddRoot(name, funcIndex, reachable, worklist)
	}

	neededIfaceMethods := make(map[string]bool)

	for {
		// BFS: scan each reachable function for call edges
		for len(worklist) > 0 {
			name := worklist[len(worklist)-1]
			worklist = worklist[0 : len(worklist)-1]

			idx, ok := funcIndex[name]
			if !ok {
				continue
			}
			f := irmod.Funcs[idx]
			if f.Native != nil {
				for _, fx := range f.Native.Fixups {
					if fx.Target != "" && !reachable[fx.Target] {
						reachable[fx.Target] = true
						worklist = append(worklist, fx.Target)
					}
				}
			}

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
					if inst.Name == "Tostring" {
						neededIfaceMethods["Error"] = true
						neededIfaceMethods["String"] = true
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
				} else if inst.Op == OP_CONST_I64 && len(inst.Name) > 10 && inst.Name[0:10] == "$funcaddr$" {
					refName := inst.Name[10:]
					if !reachable[refName] {
						reachable[refName] = true
						worklist = append(worklist, refName)
					}
				} else if inst.Op == OP_IFACE_CALL {
					methodName := dceMethodName(inst.Name)
					if methodName != "" {
						neededIfaceMethods[methodName] = true
					}
				}
			}
		}

		addedMethodRoots := false
		for key, funcName := range irmod.MethodTable {
			methodName := dceMethodName(key)
			if !neededIfaceMethods[methodName] {
				continue
			}
			if !reachable[funcName] {
				reachable[funcName] = true
				worklist = append(worklist, funcName)
				addedMethodRoots = true
			}
		}
		if !addedMethodRoots {
			break
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

	// Prune method table entries whose target function was removed.
	filteredMethods := make(map[string]string)
	for key, funcName := range irmod.MethodTable {
		if reachable[funcName] {
			filteredMethods[key] = funcName
		}
	}
	irmod.MethodTable = filteredMethods
}
