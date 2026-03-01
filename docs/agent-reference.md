# Agent Reference (Optional)

This file is supplemental to `AGENTS.md`.
Load it only when the task needs extra workflow detail.

## Curated Buildfile Targets

This list is intentionally non-exhaustive. Use `./build/build --list` for the full live set.

- `build` - build native compiler (`build/rtg`)
- `selfhost` - native 3-stage self-host stability check
- `selfhost-i386` - i386 3-stage self-host
- `selfhost-c` - C backend 3-stage self-host
- `selfhost-wasm` - WASM backend 3-stage self-host
- `test` - standard validation alias (`test-fullcompiler-rtg` + selfhost checks)
- `test-fullcompiler-rtg` - fullcompiler suite on native RTG backend
- `test-fullcompiler-rtg-i386` - fullcompiler suite targeting `linux/386`
- `test-fullcompiler-rtg-win386` - fullcompiler suite targeting `windows/386`
- `test-fullcompiler-c` - fullcompiler suite targeting `c/64`
- `test-fullcompiler-wasm` - fullcompiler suite targeting `wasi/wasm32`
- `test-size-analysis-tagsets` - backend/tag-set matrix validation
- `crosscompile-wasm-native` - wasm-built compiler cross-compiling native outputs
- `test-build` - rebuild runner and run standard test flow
- `clean` - clean generated build artifacts

## Debugging Checklist

If `selfhost*` or `test-fullcompiler*` hangs or behaves inconsistently:

1. Run `./build/build clean`
2. Rebuild runner: `go build -o build/build ./tools`
3. Retry the narrowest relevant target

## Execution Protocols

Validation protocol:

1. Run requested validation steps one-by-one (avoid giant `set -e` scripts)
2. Stop at first failure
3. Report exact failing command + concise stderr
4. Mark remaining requested steps as not run

Hang/timeout protocol:

1. For long `selfhost*` / `test-fullcompiler*` checks, use `build/withtimeout` (`tools/cmd/withtimeout/main.go`) when bounded execution is needed
2. On timeout/hang, capture useful log tail
3. Run clean + runner rebuild
4. Retry once before escalating

Requirement lock protocol:

1. Before substantial refactors or format/UI-heavy changes, restate acceptance criteria in 2-4 concrete bullets
2. If ambiguous, ask one concise clarification question before edits

## Working Notes

- Backend selection is mostly controlled by file build constraints
- Special tags like `no_backend_*` and `no_embed_std` are used in targeted flows
- Keep Buildfile targets and backend/tag assumptions aligned when adding/removing backends
