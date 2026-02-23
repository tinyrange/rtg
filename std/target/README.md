# Adding a New In-Tree Target

This directory contains RTG target registrations used by `-T <triple>`.

A target package defines:
- compile-time defaults (`GOOS`, `GOARCH`, pointer/word sizes, backend)
- optional generation logic via `target.Driver`
- optional ABI/assembler/binfmt metadata used by profile-based generation

For external single-file targets loaded with `-target-file` or `-target-root`, see `TARGET_SINGLE_FILE.md`.

## Overview of the flow

1. `std/compiler/main.go` calls `target.Apply(triple, tgt)` during `-T` resolution.
2. `std/compiler/backend/backend.go` calls `target.Generate(triple, ...)` before legacy backend switches.
3. If no driver handled generation, backend profile routing may handle known `Assembler` + `BinFormat` pairs.

## File layout

Create a package under `std/target/<os>/<arch>/`.

Typical files:
- `target.go`: target spec + directive registrations
- `generate.go`: `Driver.Generate` implementation
- `disabled.go`: optional stub when backend is build-tag disabled
- `bootstrap_gc.go`: host Go bootstrap shim for stage0 registration

## Step 1: Define and register the target spec

Create `target.go` with a target driver and directives.

```go
//go:build !no_backend_riscv64

package riscv64

import (
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
	"j5.nz/rtg/std/target"
)

const packagePath = "j5.nz/rtg/std/target/linux/riscv64"

type linuxRiscv64Driver struct{}

func (d linuxRiscv64Driver) Configure(_ *common.Target) error {
	return nil
}

func (d linuxRiscv64Driver) Generate(_ *common.Target, _ *ir.IRModule, _ string) error {
	// Implement code generation in generate.go.
	return nil
}

//rtg:target linux/riscv64
func linuxRiscv64TargetSpec() target.Spec {
	return target.Spec{
		Triple:      "linux/riscv64",
		PackagePath: packagePath,
		Defaults: target.Defaults{
			GOOS:     "linux",
			GOARCH:   "riscv64",
			PtrSize:  8,
			WordSize: 8,
			Backend:  "native",
		},
		Driver:    linuxRiscv64Driver{},
		Assembler: "riscv64",
		BinFormat: "elf64",
	}
}

//rtg:assembler riscv64
func riscv64AssemblerProvider() string { return "builtin.riscv64" }

//rtg:binfmt elf64
func elf64BinFmtProvider() string { return "builtin.elf64" }

//rtg:targetabi linux/riscv64
func linuxRiscv64ABI() target.GenericABI {
	return target.GenericABI{
		Kind: "linux/riscv64",
		U64: map[string]uint64{},
		I64: map[string]int64{},
		Str: map[string]string{},
		Bool: map[string]bool{},
	}
}
```

Notes:
- `//rtg:targetabi` may return `string`, `target.ABIProvider`, or `target.GenericABI`.
- Use `string` if you want `target.RegisterABI(...)` encoded payload compatibility.
- Use `target.GenericABI` for typed ABI maps (`U64`, `I64`, `Str`, `Bool`).

## Step 2: Implement generation

Put real emission logic in `generate.go`.

Common patterns:
- keep `Configure` small (target defaults and flags)
- keep architecture/bin-format logic in dedicated helpers
- return explicit errors for unsupported modes

If you do not provide a driver, set `Driver: nil` and ensure your target can be handled by profile-based generation in `std/compiler/backend/backend_profile_*.go`.

## Step 3: Add disabled build-tag variant (optional)

If the backend can be excluded with a build tag, add `disabled.go` with matching `//rtg:target` registration and a driver that returns a clear error from `Generate`.

For ABI in disabled mode, return `kind=none` (string) or `target.GenericABI{Kind: "none"}`.

## Step 4: Add stage0 bootstrap shim

Directive-based auto-registration is used in RTG-built stages.
Host-Go stage0 still needs manual registration.

Add `bootstrap_gc.go`:

```go
//go:build gc

package riscv64

import "j5.nz/rtg/std/target"

func init() {
	target.Register(linuxRiscv64TargetSpec())
	target.RegisterABI("linux/riscv64", "kind=linux/riscv64")
}
```

If your primary ABI is typed, keep a small encoded string for bootstrap and parse the full typed form at runtime.

## Step 5: Include the package in `std/target/all`

Add a blank import in `std/target/all/all.go`:

```go
package all

import _ "j5.nz/rtg/std/target/linux/riscv64"
```

Without this import, your in-tree target package will not auto-register in normal compiler builds.

## Validation checklist

Run the narrowest relevant targets first, then broader ones if backend/codegen changed.

Recommended baseline:
- `go build -o build/build ./tools`
- `./build/build build`
- `./build/build test`

If backend/codegen changed, also run at least one relevant self-hosting target such as:
- `./build/build selfhost`
- `./build/build selfhost-wasm`

If build-tag/backend matrix behavior changed, run:
- `./build/build test-size-analysis-tagsets`
