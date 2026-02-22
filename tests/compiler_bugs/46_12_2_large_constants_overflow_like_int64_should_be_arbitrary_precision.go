// SOURCE: ISSUES.md :: 12.2 Large constants overflow like int64 (should be arbitrary precision)
// EXPECT: pending
package main
//... Exit harness ...
const x = 1 << 63
const y = x >> 63
func main(){ Exit(uintptr(y & 255)) }
