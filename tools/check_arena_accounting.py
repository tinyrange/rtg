#!/usr/bin/env python3

import sys
from pathlib import Path


def parse_rows(path: Path):
    rows = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line or line.startswith("arena_report") or line.startswith("note=") or line.startswith("id "):
            continue
        parts = line.split()
        if len(parts) < 11:
            continue
        rows[parts[10]] = {
            "id": int(parts[0]),
            "parent": int(parts[1]),
            "depth": int(parts[2]),
            "enters": int(parts[4]),
            "allocs": int(parts[5]),
            "req": int(parts[6]),
            "mmap": int(parts[7]),
            "retained_req": int(parts[8]),
            "retained_mmap": int(parts[9]),
        }
    return rows


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: check_arena_accounting.py <report>", file=sys.stderr)
        return 2

    rows = parse_rows(Path(sys.argv[1]))
    parent = rows.get("tests.arena.accounting.parent")
    child_direct = rows.get("tests.arena.accounting.child.direct")
    child_routed = rows.get("tests.arena.accounting.child.routed")
    if parent is None or child_direct is None or child_routed is None:
        print("FAIL: missing accounting rows")
        return 1

    errors = []
    if parent["enters"] <= 0:
        errors.append("parent enters == 0")
    if child_direct["enters"] <= 0:
        errors.append("child.direct enters == 0")
    if child_routed["enters"] <= 0:
        errors.append("child.routed enters == 0")
    if parent["req"] < 1024:
        errors.append(f"parent req_bytes too small ({parent['req']})")
    if child_direct["req"] < 1024:
        errors.append(f"child.direct req_bytes too small ({child_direct['req']})")
    if child_routed["req"] >= child_direct["req"] - 1024:
        errors.append(
            f"child.routed req_bytes {child_routed['req']} not materially smaller than child.direct {child_direct['req']}"
        )
    if parent["req"] <= child_routed["req"]:
        errors.append(f"parent req_bytes {parent['req']} <= child.routed req_bytes {child_routed['req']}")

    if errors:
        print("FAIL: arena accounting mismatch")
        for err in errors:
            print(err)
        return 1

    print("PASS: arena accounting OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
