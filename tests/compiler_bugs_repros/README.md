# Compiler bug reproduction scaffolds

These files are intentionally placed under `tests/compiler_bugs_repros/` so existing top-level test runners that scan `tests/*.go` do not trigger them automatically.

Each file corresponds to one item from `COMPILER_BUGS` and preserves the problematic code shape as a minimal scaffold for manual selfhost/cross-selfhost investigation.

See also `ROOT_CAUSE_ANALYSIS.md` for compile-matrix results and likely root-cause groupings.
