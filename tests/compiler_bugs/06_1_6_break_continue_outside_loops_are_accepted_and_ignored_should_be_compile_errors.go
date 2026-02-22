// SOURCE: ISSUES.md :: 1.6 `break` / `continue` outside loops are accepted and ignored (should be compile errors)
// EXPECT: pending
package main
//rtg:internal Syscall
func Syscall(num,a0,a1,a2,a3,a4,a5 uintptr)(uintptr,uintptr,int32)
func Exit(code uintptr){ Syscall(231,code,0,0,0,0,0) }

func main(){
    break
    Exit(1)
}
