// SOURCE: ISSUES.md :: 1.4 `break label` ignored (acts like unlabeled `break`)
// EXPECT: pending
package main
//rtg:internal Syscall
func Syscall(num,a0,a1,a2,a3,a4,a5 uintptr)(uintptr,uintptr,int32)
func Exit(code uintptr){ Syscall(231,code,0,0,0,0,0) }

func main(){
outer:
    for i:=0; i<1; i++ {
        for j:=0; j<1; j++ {
            break outer
        }
        Exit(1)
    }
    Exit(2)
}
