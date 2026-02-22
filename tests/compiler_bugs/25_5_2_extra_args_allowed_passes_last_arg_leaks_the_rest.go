// SOURCE: ISSUES.md :: 5.2 Extra args allowed (passes last arg, leaks the rest)
// EXPECT: pending
package main
//... Exit harness ...
func f(a int) int { return a }
func main(){ Exit(uintptr(f(3,4))) }
