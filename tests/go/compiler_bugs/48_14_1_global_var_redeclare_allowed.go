// SOURCE: ISSUES.md :: 14.1 Global var redeclare allowed
// EXPECT: pending
package main
//... Exit harness ...
var x=1
var x=2
func main(){ Exit(uintptr(x)) }
