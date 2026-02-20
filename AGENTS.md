# AGENTS.md

This file is the source of truth for agent instructions in this repository.

RTG is a self-hosting Go compiler (`j5.nz/rtg`, Go `1.25.6`) with the main compiler in `std/compiler/`.
Primary build/test orchestration is defined in `tools/Buildfile`.

## Mandatory Rules

1. Always run `go` commands outside the sandbox.
2. Prefer Buildfile targets through the compiled runner after `test-build` creates `build/build`:
   - `./build/build <target>`
3. Do not use `go test ./...` for project validation. RTG runtime/std use `//rtg:internal` intrinsics that do not compile with the host Go toolchain.
4. Do not create Go test files (`*_test.go`) in this repository; validate behavior via Buildfile targets instead.

## Verified Buildfile Targets

- `build`
  - `go build -o build/rtg ./std/compiler/` (no explicit build tags).
- `selfhost`
  - 3-stage self-hosting on default target and `cmp` stage2 vs stage3.
- `selfhost-i386`
  - 3-stage self-hosting with `-T linux/386`.
- `selfhost-c`
  - 3-stage self-hosting with `-T c/64` and `${CC:-cc}` between stages.
- `selfhost-wasm`
  - 3-stage self-hosting with `-T wasi/wasm32` via `wasmtime`.
- `test-tagset-windows386-only`
  - Builds native stage2 then verifies the `windows/386-only` backend tag set compiles native target outputs.
- `test-size-analysis-tagsets`
  - Builds native stage2 then validates the full size-analysis tag-set matrix across native and WASM targets.
- `selfhost-win386`
  - 3-stage self-hosting with `-T windows/386` via `wine`.
- `crosscompile-wasm-native`
  - Builds compiler to WASM, then uses it to emit native `linux/amd64`, and checks stability.
- `test`
  - Alias for `test-fullcompiler-rtg`.
- `test-fullcompiler-rtg`
  - Runs top-level fullcompiler suite with RTG backend and validates output.
- `test-fullcompiler-rtg-i386`
  - Runs top-level fullcompiler suite with RTG backend targeting `linux/386`.
- `test-fullcompiler-rtg-win386`
  - Runs top-level fullcompiler suite with RTG backend targeting `windows/386` (uses `wine` on Linux hosts).
- `test-fullcompiler-c`
  - Runs top-level fullcompiler suite with `-T c/64` and validates output.
- `test-fullcompiler-wasm`
  - Runs top-level fullcompiler suite with `-T wasi/wasm32` via `wasmtime` and validates output.
- `test-build`
  - Builds `build/build`, lists targets, then runs `test`.
- `playground`
  - Builds `web/compiler.wasm` with `-tags no_embed_std`, then runs `web/build.sh`.
- `clean`
  - Removes generated build, stage, size, and cross-compile artifacts.

## Debugging Tip

- If `selfhost*` or `test-fullcompiler*` appears to hang or behaves inconsistently, run `./build/build clean` first, then rebuild the runner (`go build -o build/build ./tools`) and retry.

## Feature Workflow

For each feature/change:

1. Run the narrowest relevant `tools/Buildfile` target(s).
2. If backend/codegen behavior changed, run at least one `selfhost*` target relevant to the touched backend(s).
3. If backend build-tag constraints changed, run `test-size-analysis-tagsets`.
4. In your change summary, include which Buildfile targets were run.

## Working Notes

- Backend selection is primarily controlled by file build constraints; special tags like `no_backend_*` and `no_embed_std` are used in targeted flows.
- Keep `tools/Buildfile` targets and backend/tag assumptions aligned when adding/removing backends.
