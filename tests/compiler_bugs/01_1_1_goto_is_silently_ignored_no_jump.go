// SOURCE: ISSUES.md :: 1.1 `goto` is silently ignored (no jump)
// EXPECT: pending
package main
//rtg:internal Syscall
func Syscall(num,a0,a1,a2,a3,a4,a5 uintptr)(uintptr,uintptr,int32)
func Exit(code uintptr){ Syscall(231,code,0,0,0,0,0) }

func main(){
    goto L
    Exit(1)
L:
    Exit(2)
}
