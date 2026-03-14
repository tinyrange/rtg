# SPIKE_FIX

This file records the current source-review status of the reported issues in
`std/compiler/frontend/go` and `std/compiler/backend/x64`, plus a practical fix
order for the spike.

Status meanings:
- `Confirmed`: directly supported by current source.
- `Likely bug`: source strongly suggests incorrect behavior, but end-to-end
  runtime validation is still worth doing after the patch.
- `Confirmed waste`: clearly duplicated or avoidable work in the current code.

## Confirmed Findings

1. `ResolveModule` bare-package fallback resolves the wrong directory.
   Status: `Confirmed`
   Evidence:
   - [`std/compiler/frontend/go/frontend.go`](std/compiler/frontend/go/frontend.go) sets `entryDir := dirOfPath(entryFiles[0])`.
   - For a bare package arg where `arg != "."`, it falls back to `p.parsePackageDir(entryDir, "main")`.
   - For `foo`, `dirOfPath("foo")` is `"."`, so the fallback scans `.` instead of `foo`.
   Impact:
   - Bare local package names can silently resolve the wrong directory.
   Fix:
   - Fall back to `parsePackageDir(arg, "main")` for the bare-package case, not `entryDir`.

2. Linux x64 `"$funcaddr$"` fixups are not handled correctly.
   Status: `Likely bug`
   Evidence:
   - [`std/compiler/backend/x64/backend_x64.go`](std/compiler/backend/x64/backend_x64.go) `compileFuncAddr` appends a fixup target named `"$funcaddr$"+thunkName`.
   - [`std/compiler/backend/x64/codegen.go`](std/compiler/backend/x64/codegen.go) `ResolveLinuxCallFixups` only skips `"$rodata_header$"` and `"$data_addr$"`, then resolves everything else as a normal function rel32 target.
   - Windows explicitly special-cases `"$funcaddr$"` and defers it to PE patching in [`std/compiler/backend/x64/windows/backend_windows_x64.go`](std/compiler/backend/x64/windows/backend_windows_x64.go).
   Impact:
   - Linux x64 appears to patch an 8-byte absolute function-address placeholder as if it were a rel32 call target.
   Fix:
   - Add explicit Linux handling for `"$funcaddr$"` fixups and patch them during final image layout, analogous to the Windows path.

3. `NewCodeGen` ignores its `baseAddr` argument.
   Status: `Confirmed`
   Evidence:
   - [`std/compiler/backend/x64/codegen.go`](std/compiler/backend/x64/codegen.go) takes `baseAddr uint64`.
   - The constructor hardcodes `g.BaseAddr = 0x400000`.
   Impact:
   - Any non-default base address request is ignored.
   Fix:
   - Set `g.BaseAddr = baseAddr`.

4. Source files are read twice and the build-tag header is scanned twice.
   Status: `Confirmed waste`
   Evidence:
   - Disk path:
     - [`std/compiler/frontend/go/frontend.go`](std/compiler/frontend/go/frontend.go) `shouldIncludeFile` does `os.ReadFile(path)` and scans content.
     - `parsePackageDir` later calls `parseFile(path)`, which reads the file again.
   - Embed path:
     - [`std/compiler/frontend/go/parser_rtg.go`](std/compiler/frontend/go/parser_rtg.go) reads embedded content once for `shouldIncludeContent` and again for `parseSource`.
   - Build-tag scan duplication:
     - `shouldIncludeFile` calls `collectBuildTagsFromContent(content)` and then separately rescans the same top-of-file lines for `//go:build`.
     - `shouldIncludeContent` has the same pattern.
   Impact:
   - Real repeated frontend I/O and scanning on every source file.
   Fix:
   - Introduce a shared `fileSource` struct `{path, name, content, buildExpr, include}`.
   - Read each file once per discovery pass.
   - Parse build tags once and reuse the result for filtering and tag discovery.

5. Token text is cloned twice on the `parseFile` path.
   Status: `Confirmed waste`
   Evidence:
   - [`std/compiler/frontend/go/frontend.go`](std/compiler/frontend/go/frontend.go) `parseFile` calls `cloneTokenValues(tokens)`.
   - The parser then clones again through `stableTokenString(...)` in many AST-building paths in [`std/compiler/frontend/go/parser.go`](std/compiler/frontend/go/parser.go).
   - `parseSource` does not do the first clone, so the asymmetry is already visible in the current design.
   Impact:
   - Tokens copied once even when they never escape parsing, and AST-bound strings copied twice.
   Fix:
   - Remove `cloneTokenValues(tokens)` from `parseFile`.
   - Keep cloning only at AST escape points, or centralize ownership in one place.

6. Every function body gets extra full AST pre-walks.
   Status: `Confirmed waste`
   Evidence:
   - [`std/compiler/frontend/go/compiler.go`](std/compiler/frontend/go/compiler.go) `containsDeferStmt(node.Body)` is called before compilation.
   - [`std/compiler/frontend/go/compiler.go`](std/compiler/frontend/go/compiler.go) `compileDeferStmt` already lazily allocates `deferHeadLocal` on first real defer.
   - [`std/compiler/frontend/go/compiler.go`](std/compiler/frontend/go/compiler.go) `countFuncBodyNodes` feeds `estimateCodeCap`.
   Impact:
   - At least one redundant O(body-size) pass per function, plus another speculative capacity pass.
   Fix:
   - Remove `containsDeferStmt` pre-pass entirely.
   - Revisit `countFuncBodyNodes`; either drop it or gate it behind a large-function heuristic if it proves worthwhile.

7. Escape analysis uses linear-scan slices as maps/sets.
   Status: `Confirmed`
   Evidence:
   - [`std/compiler/frontend/go/compiler.go`](std/compiler/frontend/go/compiler.go) `escapeAliasState.get/set` linearly scan `entries`.
   - [`std/compiler/frontend/go/compiler.go`](std/compiler/frontend/go/compiler.go) `escapeNameSet.has/add` linearly scan `names`.
   Impact:
   - Escape-analysis cost can go quadratic as alias/local counts grow.
   Fix:
   - Keep the small-slice fast path, but add map fallback after a threshold.

8. x64 jump relaxation repeatedly rewrites the code buffer.
   Status: `Confirmed`
   Evidence:
   - [`std/compiler/backend/x64/codegen.go`](std/compiler/backend/x64/codegen.go) `relaxCurrentFuncJumps`:
     - shortens one jump at a time,
     - splices `g.Code` to delete bytes from the middle,
     - shifts offsets after each delete,
     - repeats until no changes remain.
   Impact:
   - Repeated data movement and offset maintenance on large functions with many jumps.
   Fix:
   - Compute shrink decisions first, then rewrite the function once and patch offsets/fixups in one pass.

9. `CompileConstStr` allocates a throwaway `[]byte` for each new literal.
   Status: `Confirmed waste`
   Evidence:
   - [`std/compiler/backend/x64/backend_x64.go`](std/compiler/backend/x64/backend_x64.go) does `append(g.Rodata, []byte(decoded)...)`.
   - That conversion allocates a temporary slice.
   Impact:
   - Extra per-literal allocation and copy on the backend path.
   Fix:
   - Grow `g.Rodata`, then `copy` the decoded bytes in place.
   - Separately, consider caching decoded escaped literals before dedup lookup.

10. x64 interface dispatch construction scales poorly.
    Status: `Confirmed`
    Evidence:
    - [`std/compiler/backend/x64/backend_x64.go`](std/compiler/backend/x64/backend_x64.go) iterates `TypeIDs`, synthesizes `typeName + ".Method"` candidates, then uses `LookupStringMapLinear`.
    - [`std/compiler/backend/becommon/common.go`](std/compiler/backend/becommon/common.go) `LookupStringMapLinear` linearly scans the map.
    - Dispatch entries are then insertion-sorted.
    Impact:
    - Roughly O(types x methods) lookup behavior plus O(n^2) sorting during backend codegen.
    Fix:
    - Precompute reverse dispatch tables once per module or once per backend run.

11. Import worklist can accumulate duplicates.
    Status: `Confirmed`
    Evidence:
    - [`std/compiler/frontend/go/frontend.go`](std/compiler/frontend/go/frontend.go) only checks `mod.Packages` before enqueueing imports from parsed packages.
    - There is no `queued` set, so shared imports can be appended more than once.
    Impact:
    - Avoidable worklist churn.
    Fix:
    - Track `queued` imports separately from `mod.Packages`.

12. `sortStrings` is insertion sort and is used in non-trivial frontend paths.
    Status: `Confirmed`
    Evidence:
    - [`std/compiler/frontend/go/frontend.go`](std/compiler/frontend/go/frontend.go) defines `sortStrings` as insertion sort.
    - It is used for file lists, paths, names, and tag presentation.
    Impact:
    - Unnecessary O(n^2) work in some frontend setup paths.
    Fix:
    - Replace with a better general-purpose sort or a hybrid thresholded implementation.

## Fix Plan

### Phase 1: Correctness bugs

1. Fix bare-package fallback in `ResolveModule`.
2. Fix Linux x64 `"$funcaddr$"` handling.
3. Fix `NewCodeGen` to honor `baseAddr`.

Validation after phase 1:
- `./build/build selfhost`
- `./build/build selfhost-wasm` if x64/common code touched in ways that could affect shared paths

### Phase 2: High-confidence frontend waste

4. Collapse file discovery/parsing onto a single source read per file.
   - Add a shared source-record path for disk and embedded files.
   - Parse the build-tag header once and reuse it.
5. Remove `cloneTokenValues(tokens)` from `parseFile`.
6. Remove `containsDeferStmt` pre-pass.
7. Re-evaluate `countFuncBodyNodes`.
   - Start by measuring without it.
   - Reintroduce a capped heuristic only if it helps.
8. Add a `queued` set for import worklist handling.
9. Replace `sortStrings` with a better sort.

Validation after phase 2:
- `./build/build selfhost`
- `./build/build selfhost-alloc-site-report`
- direct `/usr/bin/time -v ./build/stage2 -strict -o ... ./std/compiler/`

### Phase 3: Higher-risk algorithmic backend/frontend work

10. Add a map fallback to escape-analysis sets/maps after a threshold.
11. Rewrite x64 jump relaxation into a two-pass transform.
12. Remove temporary `[]byte(decoded)` allocation from `CompileConstStr`.
13. Precompute reverse method dispatch tables for interface/tostring lowering.

Validation after phase 3:
- `./build/build selfhost`
- `./build/build selfhost-wasm`
- `./build/build bench-alloc-micro-compare`
- direct `/usr/bin/time -v ./build/stage2 -strict -o ... ./std/compiler/`

## Suggested Order For Immediate Work

If the goal is fastest path to meaningful wins without losing correctness:

1. Linux x64 `"$funcaddr$"` handling
2. `ResolveModule` bare-package fallback
3. `NewCodeGen(baseAddr)`
4. single-read/single-build-tag-scan frontend path
5. remove token double-clone
6. remove `containsDeferStmt`
7. rewrite jump relaxation

That order fixes the real correctness hazards first, then the obvious frontend waste, then the largest remaining backend structural cost.
