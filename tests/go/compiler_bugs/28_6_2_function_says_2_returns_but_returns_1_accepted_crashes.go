// SOURCE: ISSUES.md :: 6.2 Function says 2 returns but returns 1 (accepted, crashes)
// EXPECT: pending
package main
//... Exit harness ...
func f()(int,int){ return 1 }
func main(){
    a,b := f()
    Exit(uintptr(a*10+b))
}
