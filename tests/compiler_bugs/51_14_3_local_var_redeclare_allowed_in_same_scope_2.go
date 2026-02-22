// SOURCE: ISSUES.md :: 14.3 Local var redeclare allowed in same scope
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    x := 1
    x := 2
    Exit(uintptr(x))
}
