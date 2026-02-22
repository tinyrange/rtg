// SOURCE: ISSUES.md :: 4.3 `var a,b = 1` compiles and duplicates RHS (!!)
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    var a,b = 1
    Exit(uintptr(a*10+b))
}
