// SOURCE: ISSUES.md :: 11.4 Rune literal validation broken (`'ab'`, `''` accepted)
// EXPECT: pending
package main
//... Exit harness ...
func main(){ Exit(uintptr('')) }
