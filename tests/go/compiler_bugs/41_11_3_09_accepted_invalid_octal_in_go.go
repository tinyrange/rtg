// SOURCE: ISSUES.md :: 11.3 `09` accepted (invalid octal in Go)
// EXPECT: pending
package main
//... Exit harness ...
func main(){ Exit(uintptr(09)) }
