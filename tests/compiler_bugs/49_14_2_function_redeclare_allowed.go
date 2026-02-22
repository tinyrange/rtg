// SOURCE: ISSUES.md :: 14.2 Function redeclare allowed
// EXPECT: pending
package main
//... Exit harness ...
func f() int { return 1 }
func f() int { return 2 }
func main(){ Exit(uintptr(f())) }
