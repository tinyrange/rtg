// SOURCE: ISSUES.md :: 18.2 Missing imports are only warnings and compilation continues (even if import is bogus)
// EXPECT: pending
package main
import "nonexistent"
func main(){}
