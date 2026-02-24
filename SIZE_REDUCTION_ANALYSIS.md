# Frontend Binary Size Analysis Notes

Date: 2026-02-24

## Goal
Quantify binary size impact from compiler codegen/front-end decisions and identify practical ways to reduce compiler binary size.

## Commands Run
- `./build/build selfhost-size-native-tags`
- `./build/build test-size-analysis-tagsets`

Primary generated dataset:
- `build/size_matrix.tsv`

## High-Level Findings
- In `full` builds, compiler code (`.total` in size analysis JSON) is dominated by backend + frontend.
- Frontend (`compiler/frontend/go`) contribution in full builds is consistently large:
  - ~32.7% to ~37.3% of code bytes depending on target.
- Backend-pruned builds shift dominance to frontend:
  - frontend rises to ~54% to ~58% of code bytes in smallest backend configs.

## Representative Numbers

### linux/amd64 (from `build/sizecheck_linux_amd64_*.json`)
- `full`
  - file: 2,904,976
  - code total: 1,492,575
  - frontend: 492,041
  - backend: 851,171
- `no_embed_std`
  - file: 1,538,232
  - code total: 1,475,595
  - frontend: 487,503
  - backend: 851,171
- smallest backend config (`backend_windows_i386_only`)
  - file: 923,264
  - code total: 890,329
  - frontend: 487,503
  - backend: 267,215

### `no_embed_std` effect
Across targets, `no_embed_std` saves about 1.35MB to 1.39MB of file size, but only about 8KB to 21KB of code bytes.

Implication: the biggest code-size wins now come from frontend/backend implementation changes, not just embedded stdlib removal.

## Top Frontend Hotspots (linux/amd64 full)
From `build/sizecheck_linux_amd64_full.json`:
- 25,770 `*Compiler.compileCallExpr`
- 18,937 `*Compiler.compileAssign`
- 11,828 `*Compiler.collectTargetDirectiveInits`
- 9,392 `*Compiler.compileForRange`
- 8,075 `*Compiler.compileFunc`
- 8,022 `CompileModule`
- 7,817 `*Compiler.compileVarDecl`
- 7,434 `*Compiler.compileSwitch`
- 7,149 `*Compiler.decodeComptimeValue`
- 7,063 `*Compiler.compileAssembledFunctions`
- 6,704 `*Lexer.scanOperator`
- 6,552 `*Compiler.resolveCallName`
- 6,503 `*Compiler.exprConcreteType`
- 6,358 `*Parser.parsePrimaryExpr`

## Frontend Category Breakdown (linux/amd64 full)
Grouped by symbol family:
- Compiler methods: 337,334
- Parser methods: 56,496
- Lexer methods: 15,266
- Preprocessor methods: 10,697
- Helpers/glue: 72,248

## Size Reduction Opportunities (frontend-focused)

1. Refactor `compileCallExpr` (largest frontend symbol) to share repeated receiver/method-call lowering paths.
- Expected impact: medium-to-high (multi-KB).

2. Refactor `compileAssign` to extract repeated metadata tracking and assignment forms into smaller helpers.
- Expected impact: medium (multi-KB).

3. Table-drive directive registration/validation in `collectTargetDirectiveInits`.
- Current implementation repeats similar validation flow for `target`, `targetabi`, `assembler`, `binfmt`.
- Expected impact: medium (multi-KB).

4. Consider feature-gating compile-time assembly/comptime support behind a build tag if acceptable.
- `decodeComptimeValue`, `compileAssembledFunctions`, `tryCompileComptimeCall`, etc. are a noticeable chunk.
- Expected impact: potentially large for minimal builds.

5. Compress branch-heavy parser/lexer hot paths (`scanOperator`, `parsePrimaryExpr`) via table-driven dispatch.
- Expected impact: small-to-medium.

## Proposed Post-Merge Experiment Order
1. `compileCallExpr` dedupe/refactor.
2. `collectTargetDirectiveInits` table-driven consolidation.
3. `compileAssign` consolidation.
4. Optional feature-tag split for assemble/comptime path.

For each change:
- Re-run `./build/build selfhost-size-native-tags`
- Re-run `./build/build test-size-analysis-tagsets`
- Compare `build/size_matrix.tsv` and top frontend symbols.
