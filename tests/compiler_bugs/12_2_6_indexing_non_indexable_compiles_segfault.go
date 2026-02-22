// SOURCE: ISSUES.md :: 2.6 Indexing non-indexable compiles → segfault
// EXPECT: pending
package main
func main(){
    x := 1
    _ = x[0]
}
