// SOURCE: ISSUES.md :: 14.3 Local var redeclare allowed in same scope
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    sum := 0
    for i := range 3 {
        sum += i
    }
    Exit(uintptr(sum))
}
