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

## 2026-03-01 A-G Sequential Passes (current branch)

Method used: `/tmp/run_mem_stage2.sh <label>` with the same 3-stage self-host loop and stage3 arena/profile capture:

- stage1: `./build/rtg -strict -o /tmp/rtg.stage1.<label> ./std/compiler/`
- stage2: `/tmp/rtg.stage1.<label> -strict -profile -o /tmp/rtg.stage2.<label>.profiled ./std/compiler/`
- stage3 measured: `RTG_ARENA_REPORT=/tmp/rtg.<label>.arena RTG_PROFILE=/tmp/rtg.<label>.profile /tmp/rtg.stage2.<label>.profiled -strict -profile -o /tmp/rtg.stage3.<label>.profiled ./std/compiler/`

Reported columns:
- `allocs`, `req_bytes`, `mmap_bytes`: root (`id=1`) from `/tmp/rtg.<label>.arena`
- `stage3_real_s`: wall time (`real`) from `/tmp/rtg.<label>.t3`

| Label | Change | allocs | req_bytes | mmap_bytes | stage3_real_s | req delta vs baseline | stage3 delta vs baseline |
|---|---|---:|---:|---:|---:|---:|---:|
| `baseline_current` | Current branch baseline | 108,591,896 | 3,688,814,008 | 3,693,658,112 | 13.88 | `0.00%` | `0.00%` |
| `itemA_inst_byref` | `Inst` helper args switched to index-based checks | 108,807,740 | 3,695,871,096 | 3,701,014,528 | 13.85 | `+0.19%` | `-0.22%` |
| `itemB_fused_matcher` | Fuse/in-line append-window matcher checks | 106,666,014 | 3,627,272,704 | 3,632,840,704 | 13.98 | `-1.67%` | `+0.72%` |
| `itemC_lazy_out` | Lazy `out` allocation in `foldSliceAppendU32LE` | 106,680,351 | 3,622,231,456 | 3,627,614,208 | 13.83 | `-1.80%` | `-0.36%` |
| `itemD_classify_flags` | Precompute matcher class flags (`byteConv`/`sliceAppend`) | 106,770,490 | 3,636,211,320 | 3,640,180,736 | 13.99 | `-1.43%` | `+0.79%` |
| `itemE_reuse_bool_scratch` | Reuse global bool scratch for class flags | 106,764,849 | 3,625,111,536 | 3,630,743,552 | 13.83 | `-1.73%` | `-0.36%` |
| `itemF_parser_unary_peek` | `parseUnaryExpr` single-peek fast path | 104,049,846 | 3,538,202,464 | 3,542,663,168 | 13.69 | `-4.08%` | `-1.37%` |
| `itemG_postfix_iterative` | `parsePostfixOps` iterative loop (no recursion) | 101,412,436 | 3,453,823,096 | 3,458,777,088 | 13.54 | `-6.37%` | `-2.45%` |

Incremental observations:

- Item A hurt memory slightly.
- Item B helped memory, hurt runtime slightly.
- Item C improved both memory and runtime vs item B.
- Item D regressed both vs item C.
- Item E recovered most of item D regression.
- Item F produced a large win in both memory and runtime.
- Item G produced another large win in both memory and runtime.

Current best in this sequence: `itemG_postfix_iterative`.

### Helped-only confirmation

After dropping the regressive classify-flag path (`itemD`/`itemE`) and keeping only helpful changes
(`itemB` + `itemC` + `itemF` + `itemG`), a fresh verification run produced:

- Label: `helped_only_final`
- `allocs`: `101,288,831`
- `req_bytes`: `3,449,550,344`
- `mmap_bytes`: `3,454,582,784`
- `stage3_real_s`: `13.64`

This is slightly better memory than `itemG_postfix_iterative` while keeping runtime improved vs baseline.
