package testing

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

const failNowSentinel = "rtg.testing.failnow"

type T struct {
	Name     string
	Verbose  bool
	HadPanic bool
	PanicMsg string
	failed   bool
}

func (t *T) Fail() {
	if t != nil {
		t.failed = true
	}
}

func (t *T) Failed() bool {
	return t != nil && t.failed
}

func (t *T) Log(args ...interface{}) {
	if t != nil && t.Verbose {
		fmt.Println(args...)
	}
}

func (t *T) Logf(format string, args ...interface{}) {
	if t != nil && t.Verbose {
		fmt.Printf(format+"\n", args...)
	}
}

func (t *T) Error(args ...interface{}) {
	t.Fail()
	fmt.Println(args...)
}

func (t *T) Errorf(format string, args ...interface{}) {
	t.Fail()
	fmt.Printf(format+"\n", args...)
}

func (t *T) FailNow() {
	t.Fail()
	panic(failNowSentinel)
}

func (t *T) Fatal(args ...interface{}) {
	t.Fail()
	fmt.Println(args...)
	panic(failNowSentinel)
}

func (t *T) Fatalf(format string, args ...interface{}) {
	t.Fail()
	fmt.Printf(format+"\n", args...)
	panic(failNowSentinel)
}

type B struct {
	Name     string
	N        int
	Verbose  bool
	HadPanic bool
	PanicMsg string
	failed   bool
	running  bool
	start    int
	elapsed  int
}

func (b *B) Fail() {
	if b != nil {
		b.failed = true
	}
}

func (b *B) Failed() bool {
	return b != nil && b.failed
}

func (b *B) Log(args ...interface{}) {
	if b != nil && b.Verbose {
		fmt.Println(args...)
	}
}

func (b *B) Logf(format string, args ...interface{}) {
	if b != nil && b.Verbose {
		fmt.Printf(format+"\n", args...)
	}
}

func (b *B) Error(args ...interface{}) {
	b.Fail()
	fmt.Println(args...)
}

func (b *B) Errorf(format string, args ...interface{}) {
	b.Fail()
	fmt.Printf(format+"\n", args...)
}

func (b *B) FailNow() {
	b.Fail()
	panic(failNowSentinel)
}

func (b *B) Fatal(args ...interface{}) {
	b.Fail()
	fmt.Println(args...)
	panic(failNowSentinel)
}

func (b *B) Fatalf(format string, args ...interface{}) {
	b.Fail()
	fmt.Printf(format+"\n", args...)
	panic(failNowSentinel)
}

func (b *B) ResetTimer() {
	if b == nil {
		return
	}
	b.elapsed = 0
	b.start = runtime.Now()
	b.running = true
}

func (b *B) StartTimer() {
	if b == nil || b.running {
		return
	}
	b.start = runtime.Now()
	b.running = true
}

func (b *B) StopTimer() {
	if b == nil || !b.running {
		return
	}
	b.elapsed = b.elapsed + (runtime.Now() - b.start)
	b.running = false
}

func (b *B) SetBytes(n int64) {
	_ = n
}

func (b *B) Elapsed() int {
	if b == nil {
		return 0
	}
	if b.running {
		return b.elapsed + (runtime.Now() - b.start)
	}
	return b.elapsed
}

func IsFailNow(v interface{}) bool {
	return runtime.Tostring(v) == failNowSentinel
}

func PanicString(v interface{}) string {
	return runtime.Tostring(v)
}

func Match(name string, pattern string) bool {
	if pattern == "" || pattern == "." || pattern == "*" {
		return true
	}
	return strings.Contains(name, pattern)
}

func ParseTestArgs() (runPattern string, benchPattern string, verbose bool) {
	i := 1
	for i < len(os.Args) {
		arg := os.Args[i]
		if arg == "-v" {
			verbose = true
			i = i + 1
			continue
		}
		if strings.HasPrefix(arg, "-run=") {
			runPattern = arg[5:len(arg)]
			i = i + 1
			continue
		}
		if arg == "-run" {
			if i+1 < len(os.Args) {
				runPattern = os.Args[i+1]
				i = i + 2
			} else {
				i = i + 1
			}
			continue
		}
		if strings.HasPrefix(arg, "-bench=") {
			benchPattern = arg[7:len(arg)]
			i = i + 1
			continue
		}
		if arg == "-bench" {
			if i+1 < len(os.Args) {
				benchPattern = os.Args[i+1]
				i = i + 2
			} else {
				i = i + 1
			}
			continue
		}
		i = i + 1
	}
	return runPattern, benchPattern, verbose
}

func BeginTest(name string, verbose bool) *T {
	return &T{Name: name, Verbose: verbose}
}

func FinishTest(t *T, name string, verbose bool) {
	if t == nil {
		return
	}
	finishTest(t, name, verbose)
}

func finishTest(t *T, name string, verbose bool) {
	r := recover()
	if r != nil {
		if IsFailNow(r) {
		} else {
			t.Fail()
			t.HadPanic = true
			t.PanicMsg = PanicString(r)
		}
	}
	if verbose {
		if t.Failed() {
			fmt.Printf("--- FAIL: %s\n", name)
		} else {
			fmt.Printf("--- PASS: %s\n", name)
		}
	}
	if t.HadPanic {
		fmt.Printf("panic in %s: %s\n", name, t.PanicMsg)
	}
}

func BeginBenchmark(name string, verbose bool) *B {
	b := &B{Name: name, Verbose: verbose}
	b.N = 1000
	return b
}

func FinishBenchmark(b *B, name string, verbose bool) {
	if b == nil {
		return
	}
	finishBenchmark(b, name, verbose)
}

func PrintBenchmarkResult(name string, n int, nsPerOp int) {
	fmt.Printf("%s\t%d\t%d ns/op\n", name, n, nsPerOp)
}

func RunTest(name string, verbose bool, fn func(*T)) (ok bool) {
	t := &T{Name: name, Verbose: verbose}
	defer finishTest(t, name, verbose)
	fn(t)
	return !t.Failed()
}

func finishBenchmark(b *B, name string, verbose bool) {
	r := recover()
	if r != nil {
		if IsFailNow(r) {
		} else {
			b.Fail()
			b.HadPanic = true
			b.PanicMsg = PanicString(r)
		}
	}
	if verbose {
		if b.Failed() {
			fmt.Printf("--- FAIL: %s\n", name)
		} else {
			fmt.Printf("--- BENCH: %s\n", name)
		}
	}
	if b.HadPanic {
		fmt.Printf("panic in %s: %s\n", name, b.PanicMsg)
	}
}

func RunBenchmark(name string, verbose bool, fn func(*B)) (ok bool) {
	b := &B{Name: name, Verbose: verbose}
	b.N = 1000
	defer finishBenchmark(b, name, verbose)
	b.ResetTimer()
	fn(b)
	b.StopTimer()
	if b.N <= 0 {
		b.N = 1
	}
	nsPerOp := b.Elapsed() / b.N
	fmt.Printf("%s\t%d\t%d ns/op\n", name, b.N, nsPerOp)
	return !b.Failed()
}

func FailAndExit(failures int) {
	fmt.Printf("FAIL\t%d failed\n", failures)
	os.Exit(1)
}

func PassAndExit(verbose bool, testsRun int, benchesRun int) {
	if verbose {
		fmt.Printf("PASS\t%d tests, %d benchmarks\n", testsRun, benchesRun)
	} else {
		fmt.Print("PASS")
	}
	os.Exit(0)
}
