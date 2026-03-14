#!/usr/bin/env python3

import argparse
import struct
from pathlib import Path


PROFILE_KIND_ALLOC = 2
PROFILE_HEADER = b"RTP2"
PROFILE_RECORD_SIZE = 16


def load_map(path: Path) -> dict[int, str]:
    mapping: dict[int, str] = {}
    with path.open("r", encoding="utf-8") as f:
        header = f.readline().strip()
        if header != "alloc_site_map_v1":
            raise ValueError(f"unexpected slice-reslice map header: {header!r}")
        for raw in f:
            line = raw.rstrip("\n")
            if not line:
                continue
            hash_raw, name = line.split(" ", 1)
            mapping[int(hash_raw, 16)] = name
    return mapping


def load_profile(path: Path) -> dict[int, int]:
    data = path.read_bytes()
    if len(data) < 8 or data[:4] != PROFILE_HEADER:
        raise ValueError("unexpected profile header")
    counts: dict[int, int] = {}
    offset = 8
    while offset + PROFILE_RECORD_SIZE <= len(data):
        method_hash, _parent_hash, value, kind = struct.unpack_from("<IIII", data, offset)
        offset += PROFILE_RECORD_SIZE
        if kind != PROFILE_KIND_ALLOC:
            continue
        counts[method_hash] = counts.get(method_hash, 0) + value
    return counts


def write_reports(counts: dict[int, int], mapping: dict[int, str], md_out: Path, tsv_out: Path, top_n: int) -> None:
    rows = []
    unmapped = 0
    for site_hash, count in counts.items():
        name = mapping.get(site_hash, "<unmapped>")
        if name == "<unmapped>":
            unmapped += count
        rows.append((count, site_hash, name))
    rows.sort(reverse=True)

    total = sum(row[0] for row in rows)

    with tsv_out.open("w", encoding="utf-8") as f:
        f.write("rank\thash\tcalls\tname\n")
        for idx, row in enumerate(rows, start=1):
            f.write(f"{idx}\t0x{row[1]:08x}\t{row[0]}\t{row[2]}\n")

    with md_out.open("w", encoding="utf-8") as f:
        f.write("# Stage2 SliceReslice Callsite Report\n\n")
        f.write(f"- total SliceReslice calls: `{total}`\n")
        f.write(f"- mapped sites: `{len(rows) - (1 if unmapped else 0)}`\n")
        f.write(f"- unmapped calls: `{unmapped}`\n\n")
        f.write("## Top Sites\n\n")
        f.write("| Rank | Calls | Hash | Site |\n")
        f.write("| --- | ---: | --- | --- |\n")
        for idx, row in enumerate(rows[:top_n], start=1):
            f.write(f"| {idx} | {row[0]} | `0x{row[1]:08x}` | `{row[2]}` |\n")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--map", required=True, dest="map_path")
    parser.add_argument("--profile", required=True, dest="profile_path")
    parser.add_argument("--md-out", required=True)
    parser.add_argument("--tsv-out", required=True)
    parser.add_argument("--top", type=int, default=50)
    args = parser.parse_args()

    mapping = load_map(Path(args.map_path))
    counts = load_profile(Path(args.profile_path))
    write_reports(counts, mapping, Path(args.md_out), Path(args.tsv_out), args.top)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
