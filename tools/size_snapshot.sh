#!/usr/bin/env bash
set -eu

if [ ! -x build/build ]; then
  go build -o build/build ./tools
fi

for f in build/stage2.size.default.json build/stage2.size.default.strip.json build/stage2.size.native_only.json build/stage2.size.native_strict.json; do
  [ -f "$f" ] && chmod u+w "$f" || true
done

./build/build selfhost-size-native-tags

printf "BINARY_SIZES\n"
for f in build/rtg.stage2.default build/rtg.stage2.default.strip build/rtg.stage2.native_only build/rtg.stage2.native_strict; do
  printf "%s,%s\n" "$f" "$(stat -f%z "$f")"
done

printf "SIZE_TOTALS\n"
for f in build/stage2.size.default.json build/stage2.size.default.strip.json build/stage2.size.native_only.json build/stage2.size.native_strict.json; do
  total=$(rg -o '"total":[0-9]+' "$f" | head -n1 | cut -d: -f2)
  funcs=$(rg -o '"name":"' "$f" | wc -l | tr -d ' ')
  printf "%s,total=%s,funcs=%s\n" "$f" "$total" "$funcs"
done
