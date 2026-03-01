# AGENTS.md

This file is the source of truth for agent instructions in this repository.

RTG is a self-hosting Go compiler (`j5.nz/rtg`, Go `1.25.6`) with the main compiler in `std/compiler/`.
Primary build/test orchestration is defined in `tools/Buildfile`.

## Core Rules

1. Always run `go` commands outside the sandbox.
2. Prefer Buildfile targets through the compiled runner:
   - `go build -o build/build ./tools` (if runner is missing/stale)
   - `./build/build <target>`
   - `./build/build --list` (discover full live target set)
3. Do not use `go test ./...` for project validation. RTG runtime/std use `//rtg:internal` intrinsics that do not compile with the host Go toolchain.
4. Do not create Go test files (`*_test.go`) in this repository; validate behavior via Buildfile targets instead.
5. For `gh` markdown/text (PR title/body/comments), never use inline `--body`; use file-based flags (`--body-file`, `--title-file`) with quoted heredocs.
6. If a command is rejected/blocked, do not retry equivalent variants. Switch strategy immediately and continue.
7. Do not use broad rollback commands (`git checkout --`, `git restore`, `git reset --hard`) unless the user explicitly asks for rollback.
8. Do not invoke `apply_patch` through `exec_command`; use the patch tool directly.

## Validation Expectations

1. Run the narrowest relevant Buildfile target(s).
2. If backend/codegen behavior changed, run at least one relevant `selfhost*` target.
3. If backend build-tag constraints changed, run `test-size-analysis-tagsets`.
4. In change summaries, include the targets that were run and pass/fail status.
5. When a compiler bug or limitation is discovered during work, record it in `COMPILER_BUGS.md` in the same branch/PR.

## Optional Reference

Load [docs/agent-reference.md](docs/agent-reference.md) only when needed for detailed workflows (curated targets, timeout/hang protocol, debugging checklist, and working notes).
