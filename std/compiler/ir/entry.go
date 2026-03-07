package ir

const DefaultEntryFunc = "main.main"

// EntryFuncName resolves the program entrypoint with compatibility fallback.
func EntryFuncName(irmod *IRModule) string {
	if irmod != nil && irmod.EntryFunc != "" {
		return irmod.EntryFunc
	}
	return DefaultEntryFunc
}

// EntryFunc resolves the entrypoint function, if present in the module.
func EntryFunc(irmod *IRModule) *IRFunc {
	if irmod == nil {
		return nil
	}
	name := EntryFuncName(irmod)
	for _, f := range irmod.Funcs {
		if f != nil && f.Name == name {
			return f
		}
	}
	return nil
}

// EntryFuncRetCount resolves the entrypoint return count with a safe default.
func EntryFuncRetCount(irmod *IRModule) int {
	if f := EntryFunc(irmod); f != nil {
		return f.RetCount
	}
	return 0
}
