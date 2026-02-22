# TODO

Track compiler fixes using `tests/compiler_bugs/manifest.txt` and `./build/build test-compiler-bugs`.

## Control-flow and branch semantics
- [x] Fix `goto` and label jump lowering.
- [x] Fix `fallthrough` in `switch`.
- [x] Fix `break` handling in `switch`.
- [x] Fix labeled `break`/`continue` targets.
- [x] Reject illegal `break`/`continue` outside loops/switch.

## Type-checking gaps
- [x] Enforce boolean conditions for `if`/`for`.
- [x] Enforce boolean operands for `&&`/`||`.
- [x] Enforce comparability/type compatibility in comparisons.
- [x] Reject dereference of non-pointers.
- [x] Reject indexing non-indexable values.
- [x] Validate `len`/`cap` argument types.
- [x] Validate `panic` arity and argument typing.

## Identifier and scope resolution
- [x] Reject undefined identifiers on read/write.
- [x] Reject undefined type names.
- [ ] Fix `if init` variable scope leakage.

## Assignment/call/return arity validation
- [x] Enforce assignment LHS/RHS arity matching.
- [x] Enforce function call argument arity.
- [x] Enforce return statement arity and signature compliance.

## Integer semantics and conversions
- [x] Fix unsigned comparison codegen.
- [x] Fix unsigned right-shift codegen.
- [x] Fix unsigned division codegen.
- [x] Enforce width truncation/wrap for `int8`/`uint8`/`int32`.
- [x] Implement missing numeric conversion handling (`uint8(x)`, `int8(x)`, etc.).

## Arrays and range behavior
- [ ] Fix local array allocation/storage semantics.
- [ ] Reject invalid `range` targets (e.g. integer).

## Lexer/parser validation
- [ ] Enforce numeric underscore placement rules.
- [ ] Reject invalid base literals and digits.
- [ ] Reject invalid octal forms like `09`.
- [ ] Enforce rune literal validity (single rune only).
- [ ] Enforce string escape validation.

## Constant evaluation
- [ ] Reject constant division by zero.
- [ ] Implement proper arbitrary-precision constant handling.

## Redeclaration rules
- [ ] Reject global variable redeclaration.
- [ ] Reject function redeclaration.
- [ ] Reject local redeclaration in same scope.
- [ ] Enforce `:=` "at least one new variable" rule.

## Defer and function values
- [ ] Fix deferred function-value calls/linking.

## Driver/import diagnostics
- [x] Ensure `-run` preserves child process exit code semantics.
- [x] Make missing imports hard compile errors.

## Regression discipline
- [ ] Keep `tests/compiler_bugs/manifest.txt` in sync as behavior changes.
- [ ] For each fixed item, update expected outcome and keep suite passing.
