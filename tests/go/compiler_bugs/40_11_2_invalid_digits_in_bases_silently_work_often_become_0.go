// SOURCE: ISSUES.md :: 11.2 Invalid digits in bases silently “work” (often become 0)
// EXPECT: pending
package main
//... Exit harness ...
func main(){ Exit(uintptr(0b2)) }  // invalid
