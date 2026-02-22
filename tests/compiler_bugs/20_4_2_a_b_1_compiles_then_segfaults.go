// SOURCE: ISSUES.md :: 4.2 `a,b := 1` compiles then segfaults
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    a,b := 1
    Exit(uintptr(a*10+b))
}
