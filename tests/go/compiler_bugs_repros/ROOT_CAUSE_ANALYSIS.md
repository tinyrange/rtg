# Compiler bug root-cause notes (repro scaffold pass)

Command used for the matrix:

```bash
for f in tests/go/compiler_bugs_repros/*.go; do
  ./build/rtg -T <target> -o /tmp/bugrepro_out/<name> "$f"
done
```

`01` was compiled with `-T c/64`, all others with `-T linux/amd64`.

## Findings by bug family

1. **Unnamed receiver parser failure (`02`)**
   - **Reproduced**.
   - Root cause: parser receiver grammar requires `(<ident> <type>)` and does not accept type-only receivers (`(T)`). `parseReceiver` immediately `expect(TOKEN_IDENT)` then parses type, which rejects valid unnamed Go receivers.

2. **Function-typed local call unresolved (`05`, `06`, `12`, `15`)**
   - **Reproduced** (`unresolved calls: fn/gen`).
   - Root cause: `resolveCallName` returns local identifier names for callable locals/function values, but backend call fixups are symbol-based direct calls and do not lower indirect/function-value calls.

3. **Interface dispatch through selector/map lookups (`08`, `16`)**
   - **Reproduced** (`unresolved calls: unknown`).
   - Root cause: unresolved selector call paths return sentinel call target `"unknown"` when concrete receiver/method cannot be statically resolved for direct-call lowering.

4. **Deterministic drift / stage mismatch class (`04`, `07`, `09`, `10`, `11`, `13`, `19`, and the drift variants in the bug list)**
   - **Not directly provable with single-file compile**, but likely same underlying issue:
   - Root cause candidate: map iteration order is emitted directly in binary-affecting paths (IR serialization and interface dispatch table generation) without key sorting; this can perturb emitted order and cause stage drift.

5. **`err.Error()` and helper-returned interface calls (`03`, `17`)**
   - Standalone scaffolds compiled in current tree, so the original failures appear to be **shape-sensitive within specific selfhost paths**, not a universal parse failure.
   - Root cause candidate: interface-call lowering and method-table state become fragile when the receiver type information is lost through helper boundaries in larger driver code.

6. **Frontend/backend interface helper crashes (`01`, `14`) and assertion path (`18`)**
   - Standalone scaffolds compiled in current tree, indicating these are **context-dependent selfhost codegen instability bugs** rather than always-on frontend parse errors.
   - Root cause candidate: interaction between interface/type-assertion helperized control flow and non-deterministic backend symbol/type metadata ordering.

## Source code evidence for the root causes

- Receiver parser requires identifier first:
  - `parseFuncDecl` + `parseReceiver` receiver flow in `std/compiler/frontend/go/parser.go`.
- Function-call name resolution returns local identifiers (not lowered indirect calls):
  - `resolveCallName` in `std/compiler/frontend/go/compiler.go`.
- Unknown selector call target is emitted on unresolved receiver type:
  - `resolveCallName` in `std/compiler/frontend/go/compiler.go`.
- Non-deterministic map iteration in binary emission:
  - `writeIRBinary` iterates maps without sorting in `std/compiler/binary/ir_binary_codec.go`.
- Non-deterministic map iteration in interface dispatch table build:
  - interface dispatch entry generation from `TypeIDs` map in `std/compiler/backend/x64/backend_x64.go`.
