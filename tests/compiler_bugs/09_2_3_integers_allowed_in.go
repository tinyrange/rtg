// SOURCE: ISSUES.md :: 2.3 Integers allowed in `&&`/`||`
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    if 1 && 0 { Exit(1) } else { Exit(2) }
}
