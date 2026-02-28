// SOURCE: ISSUES.md :: 3.3 Undefined type name accepted (defaults effectively to int storage)
// EXPECT: pending
package main
func main(){
    var x Foo
    _ = x
}
