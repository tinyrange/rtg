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
            raise ValueError(f"unexpected alloc-site map header: {header!r}")
        for raw in f:
            line = raw.rstrip("\n")
            if not line:
                continue
            hash_raw, name = line.split(" ", 1)
            mapping[int(hash_raw, 16)] = name
    return mapping


def load_profile(path: Path) -> tuple[dict[int, dict[str, int]], int, int]:
    data = path.read_bytes()
    if len(data) < 8 or data[:4] != PROFILE_HEADER:
        raise ValueError("unexpected profile header")
    stats: dict[int, dict[str, int]] = {}
    ignored = 0
    offset = 8
    while offset + PROFILE_RECORD_SIZE <= len(data):
        method_hash, parent_hash, value, kind = struct.unpack_from("<IIII", data, offset)
        offset += PROFILE_RECORD_SIZE
        if kind != PROFILE_KIND_ALLOC:
            ignored += 1
            continue
        entry = stats.setdefault(method_hash, {"calls": 0, "bytes": 0})
        entry["calls"] += 1
        entry["bytes"] += value
    return stats, ignored, offset - 8


def format_bytes(num: int) -> str:
    mib = num / (1024 * 1024)
    return f"{mib:.1f} MiB"


def write_reports(stats: dict[int, dict[str, int]], mapping: dict[int, str], md_out: Path, tsv_out: Path, top_n: int) -> None:
    rows = []
    unmapped_calls = 0
    unmapped_bytes = 0
    for site_hash, entry in stats.items():
        name = mapping.get(site_hash, "<unmapped>")
        if name == "<unmapped>":
            unmapped_calls += entry["calls"]
            unmapped_bytes += entry["bytes"]
        rows.append((entry["bytes"], entry["calls"], site_hash, name))
    rows.sort(reverse=True)

    total_calls = sum(row[1] for row in rows)
    total_bytes = sum(row[0] for row in rows)

    with tsv_out.open("w", encoding="utf-8") as f:
        f.write("rank\thash\tcalls\tbytes\tname\n")
        for idx, row in enumerate(rows, start=1):
            f.write(f"{idx}\t0x{row[2]:08x}\t{row[1]}\t{row[0]}\t{row[3]}\n")

    with md_out.open("w", encoding="utf-8") as f:
        f.write("# Stage2 Alloc Site Report\n\n")
        f.write(f"- total alloc calls: `{total_calls}`\n")
        f.write(f"- total alloc bytes: `{total_bytes}` ({format_bytes(total_bytes)})\n")
        f.write(f"- mapped sites: `{len(rows) - (1 if unmapped_calls else 0)}`\n")
        f.write(f"- unmapped calls: `{unmapped_calls}`\n")
        f.write(f"- unmapped bytes: `{unmapped_bytes}` ({format_bytes(unmapped_bytes)})\n\n")
        f.write("## Top Sites\n\n")
        f.write("| Rank | Bytes | Calls | Hash | Site |\n")
        f.write("| --- | ---: | ---: | --- | --- |\n")
        for idx, row in enumerate(rows[:top_n], start=1):
            f.write(f"| {idx} | {format_bytes(row[0])} | {row[1]} | `0x{row[2]:08x}` | `{row[3]}` |\n")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--map", required=True, dest="map_path")
    parser.add_argument("--profile", required=True, dest="profile_path")
    parser.add_argument("--md-out", required=True)
    parser.add_argument("--tsv-out", required=True)
    parser.add_argument("--top", type=int, default=50)
    args = parser.parse_args()

    mapping = load_map(Path(args.map_path))
    stats, ignored, record_bytes = load_profile(Path(args.profile_path))
    write_reports(stats, mapping, Path(args.md_out), Path(args.tsv_out), args.top)
    if ignored:
        print(f"note: ignored {ignored} non-allocation records from {record_bytes // PROFILE_RECORD_SIZE} total records")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
