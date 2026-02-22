// SOURCE: ISSUES.md :: 3.2 Assigning to undefined variable is ignored
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    y = 5
    Exit(uintptr(y))
}
