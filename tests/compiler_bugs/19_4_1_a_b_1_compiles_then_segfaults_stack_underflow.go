// SOURCE: ISSUES.md :: 4.1 `a,b = 1` compiles then segfaults (stack underflow)
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    a:=0; b:=0
    a,b = 1
    Exit(uintptr(a*10+b))
}
