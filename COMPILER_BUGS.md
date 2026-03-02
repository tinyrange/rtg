# COMPILER_BUGS.md

Compiler bugs/limitations discovered while implementing stdlib extensions (`errors`, `strconv`, `bytes`, `bufio`, `flag`, `log`) on 2026-02-27, plus session-log/shadow-tree audit findings on 2026-03-01.

## Status Snapshot (2026-03-01)

### Open
- `#14` `55_stdlib_additions_extended` crashes on RTG x64 runtime targets (`linux/amd64`, `windows/amd64`).
- `#15` WASM `iface_typeassert` still fails validation (`expected i32 but nothing on stack`).
- `#16` Parser rejects unnamed method receivers (`func (T) M()`).
- `#17` Function-valued callback/local calls still lower as unresolved `fn`.
- `#18` Function values stored in maps can fail symbol resolution (`undefined: genA`).
- `#19` Unsigned arithmetic/comparison lowering still behaves as signed (`31`/`32`/`33` corpus cases).
- `#20` Integer overflow semantics for fixed-width integers are still incorrect (`34`/`35`/`36` corpus cases).
- `#21` WASM->native bootstrap currently converges one stage late (`cross_stage3 != cross_stage4`, `cross_stage4 == cross_stage5`).

### Watch (not currently reproducible)
- `#1` ICE in `compileGlobalInits` for package-scope initializers.
- `#7` package-level `log` state runtime crash paths.

### Resolved / Not Reproduced
- `#2` interface method calls on chained struct fields.
- `#3` chained receiver-field interface call in custom wrapper types.
- `#4` type-asserted interface method dispatch.
- `#5` chained temporary method call degrading to unresolved `unknown`.
- `#6` bool-pointer deref typing in conditions/comparisons.
- `#8` top-level function values unresolved in synthetic init wiring.
- `#9` prior stdlib fixture callback-unresolved path.
- `#10` `log.Logger.SetOutput` + write runtime crash.
- `#11` deferred `testing.FinishTest`/`FinishBenchmark` panic-sentinel recovery.
- `#12` WASM validator/semantic failures in extended stdlib fixtures (`54`/`55`).
- `#13` DOS/8086 COMEMU OOM for `54_stdlib_cli_core`.
- Historical DOS map/slice COMEMU failures from logs (`map_literal`, `slice_ops`, `map_comma_ok`, `map_range`, `map_types`, `slice_append`, `slice_nested`, `slice_range`) now PASS.

## Work Order

1. `#17` Fix indirect/function-value call lowering (`fn` unresolved family).
2. `#18` Fix map-held function symbol resolution (`genA`/function values).
3. `#16` Fix parser support for unnamed method receivers.
4. `#15` Fix WASM `iface_typeassert` validator failure and remove skip.
5. `#19` and `#20` fix unsigned ops and fixed-width overflow semantics.
6. `#21` Root-cause the one-stage WASM->native convergence lag and restore `stage3 == stage4`.
7. `#14` Diagnose RTG x64 runtime crash in `55_stdlib_additions_extended`.
8. Re-audit watch items `#1` and `#7`; close or replace with narrow repros.

## Active / Watch Details

### 14) `55_stdlib_additions_extended` crashes on RTG x64 runtime targets

**Symptom**
- In CI on `linux/amd64` and `windows/amd64`, compiled `55_stdlib_additions_extended` exits non-zero without diagnostic output.
- Neighboring fixture `54_stdlib_cli_core` passes on the same jobs.

**Current mitigation**
- Fullcompiler skip remains for `55_stdlib_additions_extended` on RTG `amd64` targets.

**Local status**
- Cannot be directly executed/reproduced in this host environment; still open.

### 15) WASM `iface_typeassert` validator failure

**Symptom**
- `./build/rtg -T wasi/wasm32 tests/iface_typeassert.go` output fails in `wasmtime` with:
  - `Invalid input WebAssembly code ... type mismatch: expected i32 but nothing on stack`.

**Current mitigation**
- Fullcompiler skip remains in `tools/build.go`:
  - `backend == "wasm" && name == "iface_typeassert"`.

### 16) Unnamed receiver parser rejection (`func (T) M()`)

**Repro**
- Minimal:
  - `type T struct{}`
  - `func (T) M() {}`
- Current compiler emits parse errors starting with:
  - `expected type, got )`.

### 17) Function-valued callbacks unresolved (`fn`)

**Repros**
- `tests/compiler_bugs_repros/05_function_typed_parameter_call.go`
- `tests/compiler_bugs_repros/06_apply_restore_wrapper_drift.go`
- `tests/compiler_bugs_repros/12_native_backend_callback_helpers.go`
- Minimal:
  - `func wrap(fn func()) { fn() }`

**Failure**
- `error: ... unresolved calls: fn`.

### 18) Map-held function values misresolve (`undefined: genA`)

**Repro**
- `tests/compiler_bugs_repros/15_function_value_dispatch_map.go`.

**Failure**
- Compile error:
  - `main.main: undefined: genA`.

### 19) Unsigned ops still lowered with signed behavior

**Repros (historical corpus)**
- `tests/compiler_bugs/31_7_1_unsigned_comparison_uses_signed_condition_codes.go`
- `tests/compiler_bugs/32_7_2_unsigned_right_shift_uses_arithmetic_shift.go`
- `tests/compiler_bugs/33_7_3_unsigned_division_uses_signed_division.go`

**Current status**
- Reproduced in portable local run harness: expected `exit=1`, observed `exit=2`.

### 20) Fixed-width overflow semantics still wrong

**Repros (historical corpus)**
- `tests/compiler_bugs/34_8_1_int8_overflow_doesn_t_wrap.go`
- `tests/compiler_bugs/35_8_2_uint8_overflow_doesn_t_wrap.go`
- `tests/compiler_bugs/36_8_3_int32_overflow_doesn_t_wrap.go`

**Current status**
- Reproduced in portable local run harness: expected `exit=1`, observed `exit=2`.

### 21) WASM->native convergence lags by one stage

**Repro**
- In Linux CI, `crosscompile-wasm-native` produced stable mismatch:
  - `cross_stage3 != cross_stage4` (first visible difference at ELF header section offset / `.text` size drift).
- Additional stage converges:
  - `cross_stage4 == cross_stage5`.

**Current mitigation**
- `tools/Buildfile` `crosscompile-wasm-native` now validates `cross_stage4` vs `cross_stage5`.

**Impact**
- Indicates a remaining fixed-point issue in the wasm->native bootstrap path.

### 1) ICE in `compileGlobalInits` for package-scope initializers (watch)

**Historical symptom**
- Compiler panic: `ICE: stack not balanced at end of function`.

**Current status**
- Not reproducible in the current branch state.

### 7) Package-level `log` state runtime crash paths (watch)

**Historical symptom**
- Process exited with signal (`133`/`139`) and little/no diagnostics when exercising mutable package-level logger state paths.

**Current status**
- Not reproducible in the current branch state.

## Audit Notes (2026-03-01)

- `tests/compiler_bugs_repros` compile matrix:
  - `5/19` still reproduce (`02`, `05`, `06`, `12`, `15`).
  - `14/19` now compile in current branch state.
- `tests/compiler_bugs` manifest compile validation:
  - `54/55` expectations matched.
  - `53_14_3_local_var_redeclare_allowed_in_same_scope_4.go` currently compiles/runs (`exit=5`) despite manifest expecting `compile_error` (manifest/test-metadata drift).
- Non-compile-error corpus cases (`16` cases) were executed through a portable local harness:
  - `10` pass, `6` fail (`31`-`36`, now tracked as `#19`/`#20`).

## Workarounds From Logs (2026-03-01 Audit)

### Active in current tree
- WASM fullcompiler skip for `iface_typeassert` remains active in `tools/build.go`:
  - `backend == "wasm" && name == "iface_typeassert"` (`known wasm32 type-assertion issue`).
- RTG amd64 fullcompiler skip for `55_stdlib_additions_extended` remains active:
  - `backend == "rtg" && name == "55_stdlib_additions_extended" && targetArch == "amd64"` (`known x64 runtime instability`).
- Bootstrap/selfhost compatibility workaround in `tools/build.go` remains active:
  - local helpers (`listGoFilesInDir`, `fileExt`, `equalFoldASCII`) are used instead of stdlib APIs that previously caused selfhost compiler limitations in that file.

### Historical (logged, now fixed or retired)
- Stdlib fixture shape workarounds from the 2026-02-27 extension push (shadow tracker):
  - receiver field-call hoisting via locals (`s.r.Read` -> `r := s.r; r.Read`),
  - avoiding chained temporary method calls that lowered to `unknown`,
  - temporary reductions in direct `testing.RunTest`/`RunBenchmark` callback patterns.
  These are now tracked as resolved/not reproducible (`#2`-`#11`).
- DOS map/slice COMEMU workaround phase (temporary excludes/skips) is retired for the previously failing map/slice set; those repros now pass.
- Temporary runtime timing fallback experiments (Darwin `Now` path) were logged during root-cause isolation; the active runtime path now uses `profileNow` split supported/fallback files and no ad-hoc tracker workaround remains open for this.

## Resolution Notes (2026-03-01)

### 12) WASM fullcompiler failures on extended stdlib fixtures

**Fix summary**
- WASM `OP_CONST_I64` now respects width-8 operands and emits `i64.const`.
- Prior interface dispatch/type-assert stack-shape fixes in this branch were retained.

**Validation**
- `tests/54_stdlib_cli_core.go` PASS on `wasi/wasm32`.
- `tests/55_stdlib_additions_extended.go` PASS on `wasi/wasm32`.
- `./build/build test-fullcompiler-wasm` PASS with both `wasm/54_stdlib_cli_core` and `wasm/55_stdlib_additions_extended` executed (not skipped).

### 13) DOS/8086 COMEMU OOM on extended stdlib fixture

**Fix summary**
- DOS runtime mmap base tuned to `0xBC00` in `runtime_dos_16_mmapbase_default.go`.

**Validation**
- Direct COMEMU repro for `tests/54_stdlib_cli_core.go` now exits `0` and prints `PASS`.
- DOS skip for `54_stdlib_cli_core` removed from fullcompiler skip logic.

**Scope note**
- `55_stdlib_additions_extended` remains skipped on DOS for target capability reasons (`testing` timers), not allocator OOM.

### 15) `foldSliceAppendU64LE` can corrupt wasm-hosted crosscompile output

**Symptom**
- In `selfhost-wasm` CI, `crosscompile-wasm-native` produced a native stage2 binary that crashed on startup (`./build/cross_stage2 -h` segfault).

**Root cause**
- IR fold rewrote byte-wise 64-bit append sequences to `runtime.SliceAppendU64LE(hdr, v)`.
- The helper takes `v uintptr`; when compiler host is wasm32 (`uintptr` = 32-bit), upper 32 bits of `v` are lost.
- That truncation can corrupt emitted byte streams for non-wasm native outputs.

**Fix/workaround used**
- Keep the `u32` append fold.
- Disable the `u64` append fold rewrite for now (safe behavior), leaving original byte-wise sequence intact.
