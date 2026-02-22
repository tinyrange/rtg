// SOURCE: ISSUES.md :: 6.1 Function says 1 return but returns 2 (accepted)
// EXPECT: pending
package main
//... Exit harness ...
func f() int { return 1,2 }
func main(){ Exit(uintptr(f())) }
