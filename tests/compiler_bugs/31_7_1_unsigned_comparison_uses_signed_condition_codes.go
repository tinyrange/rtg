// SOURCE: ISSUES.md :: 7.1 Unsigned comparison uses signed condition codes
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    var x uint64 = 0
    y := x - 1 // max uint64
    if y > x { Exit(1) } else { Exit(2) }
}
