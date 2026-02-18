# Known Issues: Difficulty and Implementation Order

This plan is based on `known_issues.md` and the repro files in `tests/known_issues/*.go`.

Out of scope for current work:
- `1` channels
- `2` goroutines
- `3` select
- `6` recover
- `20` float literals
- `23` imaginary literals
- `24` float conversions
- `25` float division
- `32` complex/real/imag

Difficulty scale:
- 1: parser-level or narrowly scoped
- 2: scanner/builtin plumbing with low semantic risk
- 3: medium semantic/codegen impact
- 4: broad type/codegen/runtime impact
- 5: cross-cutting runtime + codegen architecture work

## Ranked List (in-scope)

- `35` blank import: **1/5**
- `34` import alias: **1/5**
- `12` switch init statement: **1/5**
- `16` type alias: **1/5**
- `17` local type declarations: **1/5**
- `21` binary/octal literals: **2/5**
- `22` numeric separators: **2/5**
- `19` raw string literals: **2/5**
- `11` fallthrough semantics: **2/5**
- `31` `print`/`println`: **2/5**
- `30` `new(T)`: **2/5**
- `33` `clear`: **3/5**
- `18` full slice expression: **3/5**
- `10` labels/goto: **3/5**
- `27` non-ASCII rune literals decode: **3/5**
- `26` `range` over string as runes: **4/5**
- `28` `string(int)` miscompile/early exit: **4/5**
- `29` bare return named-results miscompile: **4/5**
- `13` type switch: **4/5**
- `14` fixed arrays: **4/5**
- `15` array length inference (`[...]T`): **4/5**
- `4` defer no-op: **5/5**
- `5` panic skips defers: **5/5**
- `7` function literals ICE: **5/5**
- `8` function-value calls: **5/5**
- `9` method values: **5/5**

## Recommended Implementation Order

Phase 1: parser/scanner fast wins
- `35`, `34`, `12`, `16`, `17`, `21`, `22`, `19`

Phase 2: smaller semantic and builtin fixes
- `11`, `31`, `30`, `33`, `18`

Phase 3: control-flow + UTF-8 correctness
- `10`, `27`, `26`

Phase 4: high-risk miscompile fixes
- `29`, `28`

Phase 5: major architecture blocks
- `14`, `15`, `13`, `4`, `5`, `7`, `8`, `9`

Dependency notes:
- `15` depends on core fixed-array support from `14`.
- `5` (panic with defers) depends on meaningful defer semantics from `4`.
- `9` (method values) depends on function-value calling support from `8`, and in practice closure/function-value representation from `7`.
