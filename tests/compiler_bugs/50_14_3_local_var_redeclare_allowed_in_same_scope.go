// SOURCE: ISSUES.md :: 14.3 Local var redeclare allowed in same scope
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    var x=1
    var x=2
    Exit(uintptr(x))
}
