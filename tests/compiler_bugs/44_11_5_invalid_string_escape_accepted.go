// SOURCE: ISSUES.md :: 11.5 Invalid string escape accepted
// EXPECT: pending
package main
//... Exit harness ...
func main(){ Exit(uintptr(len("\q"))) }
