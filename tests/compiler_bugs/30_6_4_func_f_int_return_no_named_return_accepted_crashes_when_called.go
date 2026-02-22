// SOURCE: ISSUES.md :: 6.4 `func f() int { return }` (no named return) accepted, crashes when called
// EXPECT: pending
package main
//... Exit harness ...
func f() int { return }
func main(){ Exit(uintptr(f())) }
