// SOURCE: ISSUES.md :: 2.1 Non-bool `if` condition accepted
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    if 1 { Exit(1) } else { Exit(2) }
}
