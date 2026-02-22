// SOURCE: ISSUES.md :: 4.4 Assigning multi-return to one variable is accepted and uses last return
// EXPECT: pending
package main
//... Exit harness ...
func f()(int,int){ return 1,2 }
func main(){
    a := f()
    Exit(uintptr(a))
}
