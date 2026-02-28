// SOURCE: ISSUES.md :: 7.3 Unsigned division uses signed division
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    x := uint64(1) << 63
    y := x / 2
    if y == (uint64(1)<<62) { Exit(1) } else { Exit(2) }
}
