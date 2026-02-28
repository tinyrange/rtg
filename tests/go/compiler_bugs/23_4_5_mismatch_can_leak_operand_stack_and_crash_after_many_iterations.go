// SOURCE: ISSUES.md :: 4.5 Mismatch can leak operand stack and crash after many iterations
// EXPECT: pending
package main
//... Exit harness ...
func f()(int,int){ return 1,2 }
func main(){
    a:=0
    for i:=0;i<150000;i++{
        a = f() // should be compile error
    }
    Exit(uintptr(a))
}
