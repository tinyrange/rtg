# C Frontend

This package currently provides:

- C preprocessor tokenizer (`Lexer`) that emits preprocessing tokens.
- Token-stream preprocessor (`Preprocessor`) with:
  - `#define` / `#undef`
  - object-like and function-like macro expansion
  - `#if`, `#ifdef`, `#ifndef`, `#elif`, `#else`, `#endif`
  - `#include` with include search paths
  - `#pragma once`

- Parse-only frontend (`Parser`) for translation units/statements.
- A limited C99-to-IR lowering pass (`CompileUnits`) suitable for simple executable programs.

Current lowering scope is intentionally small: integer-centric functions, local/global declarations (including simple `int *p` pointers and fixed-size `int a[N]` arrays), string literal pointer decay (`"abc"`), arithmetic/comparisons, direct function calls, and core control flow (`if`, `while`, `do/while`, `for`, `switch/case/default`, `break`, `continue`, `return`). Calls to declared-only extern functions are supported for `-T c/*` targets.

Additional supported pieces in this subset:

- C-style casts for simple int/pointer type names (e.g. `(int)x`, `(int*)p`).
- `sizeof` on expressions and int/pointer type names.
- Brace initializers for fixed-size arrays (global and local), with zero-fill for omitted elements.
- Call-site argument checks for arity and scalar-vs-pointer expectations.

TODO (remaining items before broader C99 coverage):

- `goto` and user labels.
- Variadic function declarations/calls.
- `struct` / `union` / `enum` lowering and field/member expressions.
- `typedef`-driven type names in declarators/expressions.
- Floating-point literals, arithmetic, and conversions.
- Function pointers and indirect calls.
