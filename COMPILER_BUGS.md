# COMPILER_BUGS.md

Bugs/limitations encountered while implementing stdlib extensions (`errors`, `strconv`, `bytes`, `bufio`, `flag`, `log`) on 2026-02-27.

## 1) ICE in `compileGlobalInits` for some package-scope initializers

**Symptom**
- Compiler panic: `ICE: stack not balanced at end of function`
- Panic site: `std/compiler/frontend/go/compiler.go:2016` in `compileGlobalInits`.

**Pattern that triggered it**
- Package-level initialized sentinels in new stdlib packages (for example EOF-like vars) caused this during import/compile.

**Workaround used**
- Removed those package-level initialized sentinels and constructed the error value inline where needed.
- For globals that had to exist, switched to zero-value globals plus lazy initialization in helper functions.

## 2) Interface method calls on chained struct fields fail to compile

**Symptom**
- Errors like:
  - `cannot resolve selector call Read on chained receiver field rd`
  - `cannot resolve selector call Write on chained receiver field wr`
  - `cannot resolve selector call Write on chained receiver field out`
  - `assignment count mismatch: 2 variables but 1 values` (cascade).

**Repro pattern**
```go
type R interface{ Read([]byte) (int, error) }
type S struct{ r R }
func f(s *S, p []byte) { _, _ = s.r.Read(p) } // fails
```

**Workaround used**
```go
r := s.r
_, _ = r.Read(p) // works
```

## 3) Interface method call on chained receiver field in custom types fails

**Symptom**
- Error like: `cannot resolve selector call Error on chained receiver field err`.

**Repro pattern**
```go
type W struct{ err error }
func (w W) Error() string { return w.err.Error() } // fails
```

**Workaround used**
- Avoided this pattern in fixture code; removed the wrapper implementation that called `w.err.Error()` through a field chain.

## 4) Type-asserted interface method dispatch fails

**Symptom**
- Error like: `errors.Unwrap: cannot resolve selector call u.Unwrap (unknown receiver type)`.

**Repro pattern**
```go
u, ok := err.(interface{ Unwrap() error })
if ok {
	return u.Unwrap() // fails
}
```

**Workaround used**
- Stubbed `errors.Unwrap` behavior for now instead of dynamic interface unwrapping dispatch.

## 5) Chained method call on temporary result can become unresolved call `unknown`

**Symptom**
- Codegen error:
  - `error: 1 unresolved calls: unknown`
  - `codegen error: 1 unresolved calls`
- IR contained `call "unknown"`.

**Repro pattern**
```go
if bytes.NewBufferString("abc").String() != "abc" { ... } // fails
```

**Workaround used**
```go
b := bytes.NewBufferString("abc")
if b.String() != "abc" { ... } // works
```

## 6) Bool-pointer deref conditions are mis-typed

**Symptom**
- Errors:
  - `condition must be bool`
  - `invalid comparison between bool and non-bool`

**Repro patterns**
```go
if !*verbose { ... }           // fails
if *verbose == false { ... }   // fails
```

**Workaround used**
- Avoided bool-pointer deref checks in fixture condition expressions.

## 7) Package-level `log` state paths can crash at runtime

**Symptom**
- Program exits with signal (`exit 133`/`139`) and no output when using package-level logging flows that route through mutable package-level logger/output state.

**Repro pattern**
```go
import "log"
func main() { log.Print("x") } // crash observed in this branch state
```

**Workaround used**
- Kept `log.Logger` instance methods as primary supported path.
- Reduced package-level helpers to simpler direct `fmt`-based wrappers and removed fixture coverage for package-level output redirection.

## 8) Top-level function values can be unresolved in synthetic init wiring

**Symptom**
- Validation errors like:
  - `undefined: TestAdd`
  - `undefined: BenchmarkAdd`
- Triggered when synthetic `init` tried to pass top-level test/benchmark funcs as first-class values.

**Repro pattern**
```go
ok := testing.RunTest("TestAdd", verbose, TestAdd) // synthetic init path failed
```

**Workaround used**
- Switched test-runner injection to AST-generated wrapper functions per test/benchmark:
  - `__rtg_run_<TestName>(verbose bool) bool`
  - `__rtg_bench_<BenchmarkName>(verbose bool) bool`
- Synthetic `init` now calls wrappers directly, avoiding function-value passing in generated wiring.

## 9) Function-typed callback invocation can become unresolved call target (`fn`/`unknown`)

**Symptom**
- Codegen failure:
  - `error: unresolved calls: fn, unknown`
  - `codegen error: ... unresolved calls`
- Triggered while compiling a normal program that called `testing.RunTest` / `testing.RunBenchmark`.

**Repro pattern**
```go
ok := testing.RunTest("ok", false, func(t *testing.T) { t.Fail() }) // unresolved `fn`
```

**Workaround used**
- In fullcompiler fixtures, avoided direct calls to `RunTest`/`RunBenchmark`.
- Tested `testing` behavior via `BeginTest`/`FinishTest` and `BeginBenchmark`/`FinishBenchmark` helpers instead.

## 10) `log.Logger` output redirection + write can crash at runtime

**Symptom**
- Program exits with signal (`exit 133`) with no Go panic text.
- Triggered after changing a logger output destination and then writing through that logger.

**Repro pattern**
```go
var a bytes.Buffer
var b bytes.Buffer
l := log.New(&a, "p:", 0)
l.SetOutput(&b)
l.Print("x") // crash observed
```

**Workaround used**
- Avoided `SetOutput` + immediate write path in fullcompiler fixtures.
- Kept logger method coverage on a single stable output destination.

## 11) `testing.FinishTest` / `testing.FinishBenchmark` do not catch panic sentinels in deferred use

**Symptom**
- `FailNow` sentinel panic escapes and terminates program (`rtg.testing.failnow` printed, non-zero exit), even when `FinishTest`/`FinishBenchmark` are deferred.

**Repro pattern**
```go
t := testing.BeginTest("x", false)
defer testing.FinishTest(t, "x", false)
t.FailNow() // sentinel panic escapes in this branch state
```

**Workaround used**
- Avoided `FailNow`/panic-path assertions through `FinishTest` and `FinishBenchmark` in fullcompiler fixtures.
- Limited fixture coverage to stable non-panic paths (`Fail`, timers, parse/match helpers, begin/finish calls).
