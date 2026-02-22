// SOURCE: ISSUES.md :: 18.1 `-run` does not preserve program exit code
// MODE: run_flag
// EXPECT: driver_exit=0
package main
//... Exit harness ...
func main(){ Exit(7) }
