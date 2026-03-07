// SOURCE: ISSUES.md :: 8.1 `int8` overflow doesn’t wrap
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    var x int8 = 127
    x++
    if x==-128 { Exit(1) } else { Exit(2) }
}
