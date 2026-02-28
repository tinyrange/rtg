// SOURCE: ISSUES.md :: 5.3 Extra args in loop crashes (stack leak)
// EXPECT: pending
package main
//... Exit harness ...
func f(a int) int { return a }
func main(){
    for i:=0;i<150000;i++{ f(1,2) }
    Exit(0)
}
