// SOURCE: ISSUES.md :: 8.2 `uint8` overflow doesn’t wrap
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    var x uint8 = 255
    x++
    if x==0 { Exit(1) } else { Exit(2) }
}
