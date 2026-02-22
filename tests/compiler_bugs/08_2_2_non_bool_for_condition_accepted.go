// SOURCE: ISSUES.md :: 2.2 Non-bool `for` condition accepted
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    for 0 { Exit(1) }
    Exit(2)
}
