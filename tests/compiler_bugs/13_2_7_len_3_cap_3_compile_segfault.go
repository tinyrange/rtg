// SOURCE: ISSUES.md :: 2.7 `len(3)` / `cap(3)` compile → segfault
// EXPECT: pending
package main
//... Exit harness ...
func main(){ Exit(uintptr(len(3))) }
