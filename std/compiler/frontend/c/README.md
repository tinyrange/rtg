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

Current lowering scope is intentionally small: integer-centric functions, local/global declarations (including simple `int *p` pointers and fixed-size `int a[N]` arrays), string literal pointer decay (`"abc"`), arithmetic/comparisons, direct function calls, and core control flow (`if`, `while`, `do/while`, `for`, `switch/case/default`, `break`, `continue`, `return`).
