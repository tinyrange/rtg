// SOURCE: ISSUES.md :: 12.1 Constant division by zero becomes 0 (no error)
// EXPECT: pending
package main
//... Exit harness ...
const x = 1/0
func main(){ Exit(uintptr(x)) }
