// SOURCE: ISSUES.md :: 1.2 `fallthrough` does nothing
// EXPECT: pending
package main
//rtg:internal Syscall
func Syscall(num,a0,a1,a2,a3,a4,a5 uintptr)(uintptr,uintptr,int32)
func Exit(code uintptr){ Syscall(231,code,0,0,0,0,0) }

func main(){
    x := 1
    switch x {
    case 1:
        fallthrough
    case 2:
        Exit(2)
    }
}
