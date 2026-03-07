// SOURCE: ISSUES.md :: 5.1 Missing arg allowed (uses garbage/0)
// EXPECT: pending
package main
//... Exit harness ...
func f(a int) int { return a }
func main(){ Exit(uintptr(f())) }
