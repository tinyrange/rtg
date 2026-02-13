# AGENTS.md

This repository uses `tools/Buildfile` as the primary task entrypoint. Follow these rules when making changes.

## Mandatory Rules

1. Always run `go` commands outside the sandbox.
2. After each implemented feature, append a compiler size snapshot by running:
   - `./tools/compiler_sizes.sh`
3. Prefer running Buildfile targets through the project build runner (`./build/build <target>`) after `test-build` has produced `build/build`.

## Buildfile Targets

- `build`
  - Builds the main compiler binary at `build/rtg` with tags:
    - `backend_linux_amd64`
    - `backend_linux_i386`
    - `backend_c`
- `selfhost`
  - Runs 3-stage self-hosting for default target and compares stage2 vs stage3.
- `selfhost-i386`
  - Runs 3-stage self-hosting for `-T linux/386` and compares stage2 vs stage3.
- `selfhost-c`
  - Runs 3-stage self-hosting for `-T c/64`, compiling emitted C with `${CC:-cc}` between stages.
- `test`
  - Builds and runs baseline runtime tests (`stringstest`, `filepathtest`, `sorttest`, `exectest`).
- `test-i386`
  - Builds and runs i386 target tests (`hello386`, `write386`, and selected runtime tests).
- `test-build`
  - Builds the Buildfile runner (`build/build`), lists targets, and executes `test`.
- `sizes`
  - Appends one CSV row to `compiler_sizes.csv` with date, git ref, and compiler sizes.
- `clean`
  - Removes generated binaries, stages, and size-tracking artifacts.

## Feature Workflow

For each feature/change:

1. Run the narrowest relevant validation target(s) from `tools/Buildfile`.
2. If the change touches backend/codegen behavior, run at least one `selfhost*` target.
3. Run `./tools/compiler_sizes.sh` to append a size snapshot for historical tracking.
4. Include in your change summary:
   - Which target(s) were run
   - Whether size tracking was appended (`compiler_sizes.csv`)

## Notes

- Size tracking may record `NA` for variants that do not currently compile in this repo state.
- Keep build tags aligned across scripts and Buildfile targets when adding/removing backends.
