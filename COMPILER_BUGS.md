# COMPILER_BUGS

- Returning a `Backend` interface value from a helper function and then calling a method on it can fail in C selfhost with `rtg c backend error: interface dispatch failed` (repro: `selfhost-c` with `getRegisteredBackend(...) (Backend, error)` pattern).
- Methods with unnamed receiver identifiers (for example `func (T) M()`) are rejected by the parser during selfhost (`expected type, got )`), so receiver names are currently required.
- Calling `err.Error()` from some helper-returned `error` values can fail selfhost codegen with unresolved call `err.Error`, so new CLI helper paths should avoid requiring interface method calls for diagnostic printing.
- Threading `DriverOptions` as an extra parameter into helper-only paths can trigger selfhost instability (`cmp build/stage2 build/stage3` mismatch) even when behavior is unchanged, indicating a codegen nondeterminism bug in option-plumbing-sensitive call paths.
