// SOURCE: ISSUES.md :: 1.5 `continue label` ignored
// EXPECT: pending
package main
//rtg:internal Syscall
func Syscall(num,a0,a1,a2,a3,a4,a5 uintptr)(uintptr,uintptr,int32)
func Exit(code uintptr){ Syscall(231,code,0,0,0,0,0) }

func main(){
outer:
    for i:=0; i<2; i++ {
        for j:=0; j<2; j++ {
            continue outer
        }
        Exit(1)
    }
    Exit(2)
}
