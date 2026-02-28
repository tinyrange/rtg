package ir

const DefaultEntryFunc = "main.main"

// EntryFuncName resolves the program entrypoint with compatibility fallback.
func EntryFuncName(irmod *IRModule) string {
	if irmod != nil && irmod.EntryFunc != "" {
		return irmod.EntryFunc
	}
	return DefaultEntryFunc
}
