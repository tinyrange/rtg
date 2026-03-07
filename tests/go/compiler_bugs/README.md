# Compiler bug reproducers from ISSUES.md

This directory contains extracted reproducers from `ISSUES.md`.

- `manifest.txt` is the source of truth for suite execution order and expected current behavior.
- `tools/test_compiler_bugs.sh` runs all cases and stops at the first mismatch.
- Cases may include `//... Exit harness ...`; the runner replaces that line with `_exit_harness.inc` at runtime.
