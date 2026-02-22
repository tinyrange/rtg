// SOURCE: ISSUES.md :: 7.2 Unsigned right shift uses arithmetic shift
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    x := uint64(1) << 63
    y := x >> 63
    if y==1 { Exit(1) } else { Exit(2) }
}
