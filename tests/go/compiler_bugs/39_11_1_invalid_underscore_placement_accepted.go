// SOURCE: ISSUES.md :: 11.1 Invalid underscore placement accepted
// EXPECT: pending
package main
//... Exit harness ...
func main(){ Exit(uintptr(1__0)) }
