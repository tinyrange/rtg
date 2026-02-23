# Single-File Target Distribution (Design Sketch)

Goal: let a target be shipped as one Go file (for example to NDA SDK holders)
without requiring a compiler fork.

## Implemented

- `-target-file <path>`: load one or more target-definition source files at
  compiler startup before `-T` resolution.
- `-target-root <path>`: recursively load `.go` target-definition files from
  a directory tree.
- Directive registration supported in compiler init lowering:
  - `//rtg:target <triple>`
  - `//rtg:targetabi <triple>`
  - `//rtg:assembler <name>`
  - `//rtg:binfmt <name>`
- Typed ABI registry (`target.ABIProvider`) with `target.GenericABI`.
- Profile-based generation path (no custom driver object required) for:
  - `Assembler: "aarch64"` + `BinFormat: "macho64"`
  - provider aliases map to built-ins `builtin.aarch64` + `builtin.macho64`

## Requirements

- Target should load with `-T <triple>` and be declared by directives.
- Binary format logic, assembler logic, and ABI/calling-convention logic should be separable.
- Stage0 bootstrap (host-Go-built compiler) should still work.
- Target file should be able to include private constants and policy code.

## Proposed Model

1. Keep a small built-in library in compiler/runtime:
   - assemblers (x64/aarch64/i386/wasm/etc) exposed as stable helper APIs
   - binary writers (ELF/PE/MachO/COFF/raw) exposed as stable helper APIs
   - VM/comptime host bridge (`j5.nz/rtg/x/comptime`) for controlled host ops

2. Target file declares metadata and ABI hooks with directives:
   - `//rtg:target <triple>`: returns `target.Spec`
   - `//rtg:targetabi <triple>`: returns ABI config object
   - optional future directives:
     - `//rtg:assembler <name>`
     - `//rtg:binfmt <name>`
     - `//rtg:entry <triple>`

3. Compiler auto-wires directive functions in generated package init:
   - register target spec
   - register ABI config
   - keep deterministic ordering and duplicate detection

4. Runtime generation path:
   - target `Driver.Generate` can be implemented in pure Go using:
     - built-in assembler helpers
     - built-in format writer helpers
     - optional `//rtg:assemble <arch>` functions for instruction fragments
   - small private logic remains in the single target file.

## Bootstrapping Strategy

- Stage0 host-Go build does not rely on RTG directive lowering.
- The compiler auto-discovers built-in target definition files by scanning
  `std/target/**/target.go` at startup using the same loader path as
  `-target-file` and `-target-root`.
- This keeps the target definition model unified around single-file metadata
  instead of per-target `gc` bootstrap shims.

## Current Single-File Format

The loader intentionally accepts a restricted subset for determinism:

- Directive nodes must annotate top-level functions.
- Functions must be zero-arg with a single `return <expr>` statement.
- Supported expressions:
  - `//rtg:target`: `target.Spec{...}` keyed composite literal.
  - `//rtg:targetabi`: `nil` or `target.GenericABI{...}` keyed composite literal.
  - `//rtg:assembler` / `//rtg:binfmt`: string literal return.
- `Spec.Driver` is intentionally not supported in external target files; use
  `Spec.Assembler` + `Spec.BinFormat` profile routing.

Note on in-tree directive init wiring: RTG-built packages still support
directive-generated init registration. Built-in and external single-file target
loading uses the typed `target.ABIProvider` representation directly.

## NDA Distribution Example (e.g., handheld console SDK)

- Vendor ships `target_console.go` only to licensed developers.
- File contains:
  - target directive function
  - ABI directive function
  - Generate function calling built-in assembler/binfmt helpers
  - optional comptime calls into local SDK tools (paths, signing, packers)
- Upstream compiler remains unchanged; developers just place the target file in
  a package under `std/target/<vendor>/<arch>` (or future `-include` path).

## Near-Term Compiler Work

- Add optional "external target package root" search to avoid editing `std/`.
- Define a stable limited comptime API for SDK tool invocation/signing.
