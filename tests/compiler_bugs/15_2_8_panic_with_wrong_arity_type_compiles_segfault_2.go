// SOURCE: ISSUES.md :: 2.8 `panic()` with wrong arity/type compiles → segfault
// EXPECT: pending
package main
func main(){ panic(10) }
