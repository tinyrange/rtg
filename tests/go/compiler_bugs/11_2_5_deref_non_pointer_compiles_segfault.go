// SOURCE: ISSUES.md :: 2.5 Deref non-pointer compiles → segfault
// EXPECT: pending
package main
func main(){
    x := 1
    _ = *x
}
