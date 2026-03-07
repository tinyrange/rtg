// SOURCE: ISSUES.md :: 6.3 Void function returning value accepted; calling repeatedly crashes (stack leak)
// EXPECT: pending
package main
//... Exit harness ...
func f(){ return 1 }
func main(){
    for i:=0;i<150000;i++{ f() }
    Exit(0)
}
