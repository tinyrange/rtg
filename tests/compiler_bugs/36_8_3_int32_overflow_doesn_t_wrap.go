// SOURCE: ISSUES.md :: 8.3 `int32` overflow doesn’t wrap
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    var x int32 = 2147483647
    x++
    if x==-2147483648 { Exit(1) } else { Exit(2) }
}
