// SOURCE: ISSUES.md :: 1.3 `break` inside `switch` does **not** break
// EXPECT: pending
package main
//rtg:internal Syscall
func Syscall(num,a0,a1,a2,a3,a4,a5 uintptr)(uintptr,uintptr,int32)
func Exit(code uintptr){ Syscall(231,code,0,0,0,0,0) }

func main(){
    switch 1 {
    case 1:
        break
        Exit(2)
    }
    Exit(3)
}
