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
- `#22` Web playground WASI shim can emit host compiler binaries that are non-runnable (`darwin/arm64` repro).
- `#23` Parser rejects struct field tags in compiler sources (selfhost parse errors on backtick tags).
- `#24` IR-binary input compiled for host target can ICE when retargeted to `linux/arm64` (`unknown intrinsic runtime.SysWrite`).
- `#25` Selfhost-created files can ignore requested create permissions until an explicit `Chmod`.
- `#26` WASM backend stackifier depends on specific short-circuit jump shape; generic CFG branch inversion can emit invalid WASM.
- `#27` Non-nil memory-base optimization currently stabilizes only for `LOAD` from `LOCAL_ADDR`; broader forms regress selfhosting.
- `#28` Panic-propagation-check pruning is currently unsafe on `wasi/wasm32` selfhost (stage1 hits `map hash table full`).
- `#29` `OFFSET+LOAD/STORE` folding currently causes wasm one-stage selfhost lag (`stage2 != stage3`, `stage3 == stage4`) without a wasm guard.
- `#30` Non-void shared-return tail merge currently breaks wasm validation (`values remaining on stack at end of block`).
- `#31` Panic-unwind slow-path outlining currently breaks wasm validation (`not enough arguments on the stack for drop`).

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
- `#27` ARM64 operand-cache branch-edge desync (`OP_JMP_IF*`/`OP_JMP_*` could skip cache materialization and corrupt x28).
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
9. `#24` Decide whether cross-targeting from host-shaped IR binaries is supported; either hard-fail early with a clear diagnostic or lower required intrinsics for `linux/arm64`.
10. `#25` Root-cause selfhost `os.OpenFile`/`os.WriteFile` create-mode mismatch and remove chmod workarounds.
11. `#26` Make WASM stackifier robust to equivalent CFG forms (or formalize and enforce IR shape constraints in one place).
12. `#27` Revisit non-nil memory-base optimization expansion (`LEN/CAP`, `GLOBAL_ADDR`, C backend direct-load path) with proof/validation coverage.
13. `#30` Make wasm stackification tolerant of shared non-void return epilogues.
14. `#31` Make wasm stackification tolerate outlined panic-unwind slow paths (or preserve the inline form there).

### 23) Struct field tags are not accepted by parser in selfhost path

**Symptom**
- During `selfhost` flow, compiler source with struct tags (for example backtick-delimited tags in a codec struct) fails parse with:
  - `expected IDENT, got RAW_STRING(...)`.

**Impact**
- Blocks use of standard Go struct tags in compiler/stdlib code paths intended to compile under RTG.

**Current mitigation**
- Avoid struct tags in RTG-compiled sources; use explicit field names/format conversion logic instead.

### 24) Cross-targeting from host-shaped IR binary to `linux/arm64` can ICE on intrinsic lowering

**Symptom**
- Running backend-only codegen from a host-generated IR binary with retargeting:
  - `./build/stage2_exp_backend -T linux/arm64 -from-ir-binary build/selfhost_exp_native.irb -o build/selfhost_exp_native_arm64`
- Fails with:
  - `ICE: unknown intrinsic 'runtime.SysWrite' in compileCallIntrinsicArm64`.

**Impact**
- Prevents this IR-binary reuse pattern for `linux/arm64`; failure mode is an ICE instead of a user-facing diagnostic.

**Current mitigation**
- Generate IR with matching target assumptions for the intended backend, or avoid cross-targeting this IR-binary path for `linux/arm64`.

### 25) Selfhost-created files can ignore requested create permissions until `Chmod`

**Symptom**
- Programs compiled by RTG can create files with incorrect permissions even when the requested mode constant is correct.
- Repro on `darwin/arm64` selfhost binary:
  - `os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)` produced execute-only output until `os.Chmod(path, 0600)` was applied.
  - `os.WriteFile(path, data, 0600)` showed the same behavior.

**Impact**
- Broke text-IR fixed-point self-hosting after switching `.irt` output creation to `0600` at open time.
- Any selfhosted tool relying on initial create permissions can emit unreadable or incorrectly executable files.

**Current mitigation**
- Normalize final permissions with `os.Chmod` after close in affected compiler output paths.

### 26) WASM stackifier depends on specific short-circuit jump adjacency

**Symptom**
- A backend-independent CFG optimization that rewrites:
  - `JMP_IF/JMP_IF_NOT <L_then>; JMP <L_else>; LABEL <L_then>`
  into:
  - inverted conditional jump to `<L_else>` with fallthrough
- can break WASM self-hosting with validator error:
  - `Invalid input WebAssembly code ... control frames remain at end of function body or expression`.

**Impact**
- Prevents enabling this otherwise-valid CFG rewrite for WASM targets until stackification handles the equivalent form.

**Current mitigation**
- In `ir/opt_ir.go`, skip `foldConditionalJumpOverUnconditionalJump` when target is `wasi/wasm32`.

### 27) Broader non-nil memory-base lowering can break selfhosting

**Symptom**
- During size optimization work, enabling backend consumption of the non-nil marker more broadly caused bootstrap regressions:
  - `selfhost` stage2 compiler crashed (`Segmentation fault: 11`) on `darwin/arm64`.
- The same root issue also made some stage2 builds silently elide nil guards for unannotated `LOAD` operations, producing smaller-but-incorrect output and runtime crashes on nil dereference paths.

**Root cause**
- In selfhosted compiler builds, direct checks like:
  - `inst.Name == ir.InstNonNilMemoryBase`
  could be lowered incorrectly in backend package code, effectively comparing against an empty-string header instead of `"$nonnull_base$"` in some cases.
- This can cause unannotated instructions (`inst.Name == ""`) to be treated as non-nil hints.

**Current mitigation**
- Avoid direct imported-const comparison in backend packages:
  - use `ir.IsNonNilMemoryBase(inst.Name)` (comparison stays in package `ir`).
- Keep conservative annotation production in IR:
  - annotate only `OP_LOAD`, and only when the proof stays conservative across control-flow joins.
- C backend remains conservative for now.

### 27) ARM64 operand-cache branch edges could desync virtual vs hardware value stack (resolved)

**Symptom**
- Re-enabling ARM64 two-entry operand cache (`x26/x27`) made `selfhost` fail on stage1 execution with `EXC_BAD_ACCESS`.
- `lldb` showed faults at generated `ldr x0, [x28]` with invalid `x28`.

**Root cause**
- Conditional IR jumps (`OP_JMP_IF`, `OP_JMP_IF_NOT`, and compare-jump forms) emitted `B.cond` without forcing cache materialization first.
- Taken edges jumped directly to labels and skipped compile-time `Flush()` code emitted on fallthrough paths, so cache-resident operands were never written to the hardware value stack.

**Resolution**
- Flush before emitting ARM64 conditional branches for IR jump ops:
  - in `compileInstArm64` for `OP_JMP_IF` / `OP_JMP_IF_NOT`,
  - in `compileCompareJumpArm64`.
- Also moved cache register initialization behind a tagged helper/stub so `no_backend_arm64` builds continue to compile.

### 28) Panic-propagation-check pruning currently requires a wasm guard

**Symptom**
- A frontend IR pass that prunes `runtime.PanicShouldUnwind` checks after calls proven non-panicking causes `selfhost-wasm` stage1 failure:
  - `map hash table full`
  - build target exits with status `2`.

**Impact**
- Prevents enabling this optimization uniformly for wasm-targeted compiler builds today.

**Current mitigation**
- Guard the pruning pass when compiling for `GOARCH=wasm32`; keep conservative panic checks on wasm.

### 29) `OFFSET+LOAD/STORE` fold currently requires a wasm guard

**Symptom**
- Enabling backend-independent `OFFSET+LOAD/STORE` fusion for all targets made `selfhost-wasm` lose fixed-point at stage2:
  - `build/stage2.wasm != build/stage3.wasm`
  - extra stage converges:
    - `build/stage3.wasm == build/stage4.wasm`.

**Impact**
- Breaks strict wasm selfhost convergence expectations (`stage2 == stage3`) for this optimization.

**Current mitigation**
- Skip this fold in `ir/opt_ir.go` for `wasi/wasm32` targets until wasm path reaches stage2 fixed-point with the transform enabled.

### 30) Non-void return tail-merge currently requires a wasm guard

**Symptom**
- Enabling IR shared-return tail merge for functions with non-zero `RetCount` causes `selfhost-wasm` validator failure:
  - `Invalid input WebAssembly code ... type mismatch: values remaining on stack at end of block`.

**Impact**
- Prevents applying backend-independent non-void return-epilogue merging uniformly to wasm targets.

**Current mitigation**
- In `ir/opt_ir.go`, allow non-void return tail merge for non-wasm targets, but skip it for `wasi/wasm32` until wasm stackification handles the transformed CFG shape.

### 31) Panic-unwind slow-path outlining currently requires a wasm guard

**Symptom**
- Outlining panic-unwind slow paths after `runtime.PanicShouldUnwind` checks causes wasm validation to fail during selfhost/web compiler builds:
  - `WebAssembly.compile(): ... not enough arguments on the stack for drop`
  - `wasmtime`: `type mismatch: expected a type but nothing on stack`.

**Impact**
- Prevents using the smaller outlined panic-propagation CFG shape for `wasi/wasm32` compiler builds today.

**Current mitigation**
- Preserve the original inline `JMP_IF_NOT + DROP* + JMP` panic-unwind sequence for `wasi/wasm32`; use the outlined slow-path labels only on non-wasm targets.

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

### 22) Web playground WASI shim native compiler output not reliably runnable

**Repro (2026-03-02)**
- Using `web/compiler.wasm` with `web/wasi.js` in Node to compile `std/compiler` for `darwin/arm64` produced a Mach-O output (`build/web_stage1`) that exits with `137` when executed.
- Host-native compile of the same target from `./build/rtg` runs normally on the same machine.

**Current mitigation**
- CI web smoke test now stays entirely inside WASM-in-Node:
  - self-compile compiler to `wasi/wasm32`,
  - run that generated compiler in WASM,
  - compile and execute a WASM `PASS` smoke program.

**Scope**
- Confirmed on local `darwin/arm64` during workflow implementation; not yet triaged across other host targets.

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
