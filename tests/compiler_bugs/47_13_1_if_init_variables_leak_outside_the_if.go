// SOURCE: ISSUES.md :: 13.1 `if init` variables leak outside the if
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    if x := 1; true { _ = x }
    Exit(uintptr(x))
}
