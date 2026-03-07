// SOURCE: ISSUES.md :: 10.1 Local array variable not allocated (writes via nil)
// EXPECT: pending
package main
//... Exit harness ...
func main(){
    var a [2]int
    a[0]=1
    a[1]=2
    Exit(uintptr(a[0]*10+a[1]))
}
