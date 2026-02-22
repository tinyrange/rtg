# TODO

Track compiler fixes using `tests/compiler_bugs/manifest.txt` and `./build/build test-compiler-bugs`.

## Control-flow and branch semantics
- [x] Fix `goto` and label jump lowering.
- [x] Fix `fallthrough` in `switch`.
- [x] Fix `break` handling in `switch`.
- [ ] Fix labeled `break`/`continue` targets.
- [ ] Reject illegal `break`/`continue` outside loops/switch.

## Type-checking gaps
- [ ] Enforce boolean conditions for `if`/`for`.
- [ ] Enforce boolean operands for `&&`/`||`.
- [ ] Enforce comparability/type compatibility in comparisons.
- [ ] Reject dereference of non-pointers.
- [ ] Reject indexing non-indexable values.
- [ ] Validate `len`/`cap` argument types.
- [ ] Validate `panic` arity and argument typing.

## Identifier and scope resolution
- [ ] Reject undefined identifiers on read/write.
- [ ] Reject undefined type names.
- [ ] Fix `if init` variable scope leakage.

## Assignment/call/return arity validation
- [ ] Enforce assignment LHS/RHS arity matching.
- [ ] Enforce function call argument arity.
- [ ] Enforce return statement arity and signature compliance.

## Integer semantics and conversions
- [ ] Fix unsigned comparison codegen.
- [ ] Fix unsigned right-shift codegen.
- [ ] Fix unsigned division codegen.
- [ ] Enforce width truncation/wrap for `int8`/`uint8`/`int32`.
- [ ] Implement missing numeric conversion handling (`uint8(x)`, `int8(x)`, etc.).

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
- [ ] Ensure `-run` preserves child process exit code semantics.
- [ ] Make missing imports hard compile errors.

## Regression discipline
- [ ] Keep `tests/compiler_bugs/manifest.txt` in sync as behavior changes.
- [ ] For each fixed item, update expected outcome and keep suite passing.
