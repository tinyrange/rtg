# MEMORY_OPTIMISATION

This document records stage2 compiler memory-allocation experiments run against `main`.

## Method

For each experiment label, run:

1. `./build/build build`
2. `./build/rtg -strict -o /tmp/rtg.stage1.<label> ./std/compiler/`
3. `/tmp/rtg.stage1.<label> -strict -profile -o /tmp/rtg.stage2.<label>.profiled ./std/compiler/`
4. `RTG_ARENA_REPORT=/tmp/rtg.<label>.arena RTG_PROFILE=/tmp/rtg.<label>.profile /tmp/rtg.stage2.<label>.profiled -strict -profile -o /tmp/rtg.stage3.<label>.profiled ./std/compiler/`

Metrics are read from arena root node (`id=1`) in `/tmp/rtg.<label>.arena`:
- `req_bytes` (total requested allocation bytes)
- `mmap_bytes` (total mmap growth bytes)
- `allocs` (allocation call count)

## Baseline

- Label: `baseline`
- `req_bytes`: `5,042,101,704`
- `mmap_bytes`: `5,045,747,712`
- `allocs`: `166,141,134`

## Experiments (5 hotspot areas)

| Item | Label | Change | req_bytes delta vs baseline | mmap_bytes delta vs baseline | allocs delta vs baseline | Result |
|---|---|---|---:|---:|---:|---|
| 1 | `item1_iterative_walkers` | Frontend walker recursion removal: `validateNode` + `containsDeferStmt` iterative traversal | `-445,906,960` (`-8.84%`) | `-445,644,800` (`-8.83%`) | `-15,598,006` (`-9.39%`) | Good |
| 2 | `item2_parser_lexer_loops` | Parser/Lexer hot-loop cleanup: tighter `scanIdent`, `scanNumber`, cheaper `Parser.advance` | `-779,516,136` (`-15.46%`) | `-779,091,968` (`-15.44%`) | `-27,176,290` (`-16.36%`) | Good |
| 3 | `item3_struct_lookup_cache` | `lookupStructTypeNode` caching attempt | `-765,818,384` (`-15.19%`) | `-765,460,480` (`-15.17%`) | `-26,710,433` (`-16.08%`) | Regressed vs item2 |
| 4 | `item4_arm64_code_reserve` | ARM64 code buffer pre-reserve attempt | `-763,218,736` (`-15.14%`) | `-762,314,752` (`-15.11%`) | `-26,623,526` (`-16.02%`) | Regressed vs item2 |
| 5 | `item5_ir_dce_methodname` | IR pass allocation cleanup: remove `strings.Split` in `dceMethodName` | `-763,266,768` (`-15.14%`) | `-762,298,368` (`-15.11%`) | `-26,634,307` (`-16.03%`) | Small improvement vs item4 |

## Item 5 rejected sub-attempt

- Label: `item5_ir_pass_allocs`
- Attempted replacing label maps in `removeUnreachableIRCode` with dense arrays.
- Outcome: much worse memory (`req_bytes` only `-2.34%` vs baseline; huge regression vs item2).
- Action: reverted this sub-attempt.

## Best measured bundle

After reverting regressive item 3 and item 4 changes, and keeping item 1 + item 2 + item 5:

- Label: `item1_item2_item5_bundle`
- `req_bytes`: `4,262,368,992` (`-779,732,712`, `-15.46%` vs baseline)
- `mmap_bytes`: `4,265,607,168` (`-780,140,544`, `-15.46%` vs baseline)
- `allocs`: `138,953,212` (`-27,187,922`, `-16.36%` vs baseline)

## Files changed in kept bundle

- `std/compiler/frontend/go/frontend.go` (item 1)
- `std/compiler/frontend/go/compiler.go` (item 1)
- `std/compiler/frontend/go/parser.go` (item 2)
- `std/compiler/ir/dce.go` (item 5)

## Raw result log

- `build/memory_experiments/results.tsv`
- Arena reports per run: `build/memory_experiments/*.arena.report`
