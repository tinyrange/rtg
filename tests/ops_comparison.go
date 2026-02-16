package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Int comparisons
	if !(1 == 1) { fmt.Printf("FAIL: 1==1\n"); passed = false }
	if !(1 != 2) { fmt.Printf("FAIL: 1!=2\n"); passed = false }
	if !(1 < 2) { fmt.Printf("FAIL: 1<2\n"); passed = false }
	if !(2 > 1) { fmt.Printf("FAIL: 2>1\n"); passed = false }
	if !(1 <= 1) { fmt.Printf("FAIL: 1<=1\n"); passed = false }
	if !(1 <= 2) { fmt.Printf("FAIL: 1<=2\n"); passed = false }
	if !(2 >= 2) { fmt.Printf("FAIL: 2>=2\n"); passed = false }
	if !(2 >= 1) { fmt.Printf("FAIL: 2>=1\n"); passed = false }

	// String comparisons
	if !("abc" == "abc") { fmt.Printf("FAIL: str==\n"); passed = false }
	if !("abc" != "def") { fmt.Printf("FAIL: str!=\n"); passed = false }
	if !("abc" < "abd") { fmt.Printf("FAIL: str<\n"); passed = false }
	if !("abd" > "abc") { fmt.Printf("FAIL: str>\n"); passed = false }

	// Bool comparisons
	if !(true == true) { fmt.Printf("FAIL: bool==\n"); passed = false }
	if !(true != false) { fmt.Printf("FAIL: bool!=\n"); passed = false }

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
