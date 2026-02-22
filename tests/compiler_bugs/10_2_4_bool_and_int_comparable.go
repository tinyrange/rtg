// SOURCE: ISSUES.md :: 2.4 `bool` and `int` comparable (!)
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    if 1 == true { Exit(1) } else { Exit(2) }
}
