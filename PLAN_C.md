# C Frontend Plan

## Goals

1. Type correctness foundation
2. Declarator/type-system expansion
3. Struct/union/enum + member access
4. Function model expansion
5. Test matrix expansion

## Milestones

1. M1: Integer scalar typing and semantics
   - Add scalar type model (`char/short/int/long`, signed/unsigned).
   - Make `sizeof(type)` / `sizeof(expr)` use scalar type info.
   - Apply explicit C cast semantics for integer scalar casts.
   - Use pointee element size for pointer arithmetic and pointer dereference/index load/store where tracked.
   - Keep unsupported areas as explicit diagnostics (no silent fallback).

2. M2: Declarators and typedefs
   - Support more declarator forms and typedef-driven names.
   - Add parse/lower diagnostics for still-unsupported complex forms.

3. M3: Aggregate types
   - Introduce `struct`/`union`/`enum` representation and field/member lowering.
   - Implement layout rules needed by the supported subset.

4. M4: Calls and ABI surface
   - Add function pointer declarations and indirect calls.
   - Add variadic declaration/call support for key libc-style patterns.

5. M5: Test coverage
   - Add lex/pp/parse fixture directories and snapshot tests.
   - Expand run tests for type conversions, pointer semantics, and diagnostics.
   - Keep `test-c-frontend` green in CI with deterministic expectations.

