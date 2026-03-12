package arena

// Enter starts a compiler arena scope. Host-Go builds treat this as a no-op;
// the RTG compiler lowers selected calls to runtime arena intrinsics.
func Enter(name string) {
}

// Leave ends the current compiler arena scope.
func Leave() {
}

// RetainCurrent promotes the current compiler arena scope to its parent.
func RetainCurrent() {
}

// UseParent routes subsequent allocations to the parent arena scope.
func UseParent() {
}

// Restore restores the previous arena allocation target after UseParent.
func Restore() {
}
