# KNOWN_LIMITATIONS

The following language/runtime areas are intentionally out of scope for current RTG work.

## Concurrency
- `1` channels
- `2` goroutines
- `3` `select`

## Panic/Recover Semantics
- `6` full `recover` semantics (currently only minimal/stub behavior; no full unwind integration)

## Floating-Point and Complex Numbers
- `20` float literals
- `23` imaginary literals
- `24` float conversions
- `25` float division
- `32` `complex`/`real`/`imag`

## Repro Files
Out-of-scope repro programs are kept in:
- `/Users/joshua/dev/projects/rtg/tests/limitations/`

## Additional Deferred Repros
There are currently no additional deferred repros for the strict fullcompiler
`PASS` output contract.
