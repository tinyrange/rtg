# CLAUDE.md

## Project Overview

RTG is a self-hosting Go compiler. It compiles a subset of Go to native binaries (ELF, Mach-O, PE32), C source, and WebAssembly. The compiler lives in `std/compiler/` and is itself compiled by RTG.

Module: `j5.nz/rtg` (Go 1.25.6, zero dependencies)

## Architecture

```
std/compiler/   - Compiler: parser, IR, backends, binary format generators
std/runtime/    - Platform-specific runtime (syscalls, memory, intrinsics)
std/fmt|io|os|path|sort|strings|embed/ - Standard library
tests/          - Test programs (t00-t07, stringstest, exectest, etc.)
tools/          - Buildfile, build runner, size tracking
web/            - Browser playground (WASM-based)
build/          - Build output (gitignored)
```

### Compilation Pipeline

1. **Frontend** (`frontend.go`, `parser.go`) — lexing, parsing, module resolution
2. **IR generation** (`ir.go`) — stack-machine IR with ~40 opcodes, dead code elimination (`dce.go`)
3. **Backend codegen** (`backend.go` + `backend_*.go`) — platform-specific machine code or C source
4. **Binary output** — ELF (`elf_*.go`), Mach-O (`macho_aarch64.go`, `codesign.go`), PE32 (`pe32.go`), WASM (`wasm_module.go`)

### Supported Targets

| Target | Backend file | Binary format |
|--------|-------------|---------------|
| linux/amd64 | `backend_x64.go` | ELF |
| linux/386 | `backend_i386.go` | ELF |
| darwin/arm64 | `backend_aarch64.go` | Mach-O |
| windows/386 | `backend_i386.go` | PE32 |
| wasi/wasm32 | `backend_wasm32.go` | WASM |
| c/16, c/32, c/64 | `backend_c.go` | C source |

### Build Tags

The Buildfile `build` target uses `go build` with **no build tags** — backends are selected at compile time based on Go filename build constraints. Build tags are only needed for special cases:
- `no_embed_std` — disables embedded stdlib (used for the WASM playground)

## Commands

```sh
# Build the compiler (no build tags needed)
go build -o build/rtg ./std/compiler/

# Build the build runner, then use it
go build -o build/build ./tools/ && ./build/build test

# Self-hosting verification (stage2 and stage3 must be byte-identical)
./build/build selfhost    # linux/amd64 (default)
./build/build selfhost-c  # C backend (works on any host)

# Compile a test program
./build/rtg -o build/testbin -T darwin/arm64 tests/t00/main.go

# Run a program directly (compile + execute, auto-detects host platform)
./build/rtg -run tests/t00/main.go
echo 'package main ...' | ./build/rtg -run

# Track compiler sizes after changes
./tools/compiler_sizes.sh
```

**Note:** `go test ./...` does NOT work — RTG's std packages use `//rtg:internal` intrinsics that the Go compiler can't handle. Use `./build/build test` or `./build/build selfhost-c` instead.

### Buildfile Targets

`build`, `selfhost`, `selfhost-i386`, `selfhost-c`, `selfhost-wasm`, `test`, `test-i386`, `test-build`, `sizes`, `playground`, `clean`

## RTG Language Subset

RTG compiles a subset of Go. These are **not supported**:
- `int64()` type conversions — RTG treats these as unknown function calls
- Garbage collection — memory is explicit (arena-based)
- Full standard library — only the packages in `std/` are available

Compiler intrinsics are declared with `//rtg:internal FuncName` directives in runtime files.

## Platform-Specific Gotchas

These are real issues that have caused bugs in this project before:

### macOS (darwin/arm64)
- **ASLR breaks absolute addresses** — always use PC-relative addressing (ADRP/ADR), never literal pool loads with absolute addresses
- **Mach-O code signature embeds the output filename** — when comparing stage2/stage3 binaries, write both to the same path and rename afterward
- **Mach-O load commands must be 8-byte aligned** — cmdsize must be a multiple of 8
- **Use non-lazy binding** (not lazy) unless you implement the full `dyld_stub_binder` infrastructure
- **macOS ARM64 syscalls use `svc #0x80`** for the Unix trap, not `svc #0`
- **BSD utilities differ from GNU** — `head -n -N` doesn't work on macOS

### Windows
- **`\r\n` line endings** — the parser/lexer must handle `\r` before `\n` (Git checks out CRLF on Windows)
- **1MB default stack** — much smaller than Linux/macOS 8MB; deep recursion (e.g. self-hosting the C backend) can segfault
- **No `cc` command** — use `gcc` (MinGW) on Windows

### Go Build System
- **Implicit filename build constraints** — files ending in `_arm64.go` are silently excluded on non-arm64 platforms, even with explicit `//go:build` tags. Use `aarch64` in filenames instead of `arm64` to avoid this.
- **Internal package restriction** — `internal/` packages cannot be imported from outside the module. Don't try to write standalone test programs that import them; write tests inside the package instead.

## Coding Guidelines

### Before Writing Code
- Read the file before editing — understand existing patterns
- Check actual function signatures before using them — don't guess API names
- For `rtg.go` (the main compiler file): it's ~50k tokens. Always use offset/limit when reading

### When Making Changes
- If changing a function signature, grep for ALL callers and update them in the same change
- Run `go build ./...` after edits to catch compile errors early
- Run `./build/build selfhost-c` to verify self-hosting before declaring anything complete

### Testing
- **Always test end-to-end** — IR-level test passing does NOT mean the binary works. Compile and run the actual binary.
- When a compiled binary "hangs", it's usually a codegen bug (jumping to unresolved address, wrong syscall, ASLR issue) — not slowness
- Don't use `defs.SYS_*` constants directly for cross-platform code — use the platform-agnostic wrappers in `std/runtime/`

### Self-Hosting Verification
- Stage 2 and stage 3 binaries must be byte-identical
- On macOS, use the same output path for both stages to avoid code signature filename differences
- Run `./build/build selfhost` to verify

### What NOT to Do
- Don't use raw syscalls on macOS/Windows — use dynamic linking through libSystem/kernel32
- Don't assume Go AST types map 1:1 to RTG's custom parser types
- Don't create test `.go` files in the project root — they conflict with the module's package declarations
- Don't use `go run` for RTG programs — use `./build/rtg -run` instead (supports stdin piping and file args)
