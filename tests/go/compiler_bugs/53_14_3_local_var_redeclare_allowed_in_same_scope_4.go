// SOURCE: ISSUES.md :: 14.3 Local var redeclare allowed in same scope
// EXPECT: pending
package main
//... Exit harness ...
func f(){ Exit(5) }
func main(){
    g := f
    defer g()
}
