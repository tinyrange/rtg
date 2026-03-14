#!/usr/bin/env python3

import argparse
import re
from dataclasses import dataclass
from pathlib import Path


@dataclass
class ArenaRow:
    name: str
    req: int
    retained_req: int
    direct_reclaim_req: int
    subtree_reclaim_req: int
    mmap: int
    direct_reclaim_mmap: int
    subtree_reclaim_mmap: int
    enters: int
    allocs: int


@dataclass
class ProfilePath:
    path: str
    bytes: int
    calls: int
    depth: int


def parse_arena_tsv(path: Path) -> tuple[list[ArenaRow], int, int]:
    lines = path.read_text(encoding="utf-8").splitlines()
    rows: list[ArenaRow] = []
    total_req = 0
    total_mmap = 0
    for i, line in enumerate(lines):
        if i == 0 or not line:
            continue
        parts = line.split("\t")
        if len(parts) < 13:
            continue
        row = ArenaRow(
            name=parts[12],
            enters=int(parts[2]),
            allocs=int(parts[3]),
            req=int(parts[4]),
            retained_req=int(parts[5]),
            direct_reclaim_req=int(parts[6]),
            subtree_reclaim_req=int(parts[7]),
            mmap=int(parts[8]),
            direct_reclaim_mmap=int(parts[10]),
            subtree_reclaim_mmap=int(parts[11]),
        )
        rows.append(row)
        if row.name == "<root>":
            total_req = row.req
            total_mmap = row.mmap
    return rows, total_req, total_mmap


def parse_profile_paths(path: Path) -> tuple[list[ProfilePath], list[ProfilePath], int]:
    lines = path.read_text(encoding="utf-8").splitlines()
    start = None
    total_alloc = 0
    for i, line in enumerate(lines):
        if line.startswith("Allocation report"):
            start = i
        if line.startswith("alloc_samples="):
            parts = line.split()
            for part in parts:
                if part.startswith("total_alloc_bytes="):
                    total_alloc = int(part.split("=", 1)[1])
    if start is None:
        return [], [], total_alloc

    pat = re.compile(
        r"^(?P<prefix>(?:\|  |   )*)(?:\|- |\\- )(?P<name>.+?) bytes=(?P<bytes>\d+) calls=(?P<calls>\d+) avg="
    )
    stack: list[str] = []
    roots: list[ProfilePath] = []
    paths: list[ProfilePath] = []
    for line in lines[start + 2 :]:
        m = pat.match(line)
        if not m:
            break
        depth = len(m.group("prefix")) // 3
        name = m.group("name")
        while len(stack) > depth:
            stack.pop()
        full = stack + [name]
        item = ProfilePath(
            path=" -> ".join(full),
            bytes=int(m.group("bytes")),
            calls=int(m.group("calls")),
            depth=depth,
        )
        paths.append(item)
        if depth == 0:
            roots.append(item)
        stack = full
    return roots, paths, total_alloc


def fmt_mb(value: int) -> str:
    return f"{value / (1024 * 1024):.1f}"


def fmt_pct(part: int, total: int) -> str:
    if total <= 0:
        return "0.0%"
    return f"{(100.0 * part) / total:.1f}%"


def write_report(
    out: Path,
    arena_rows: list[ArenaRow],
    total_req: int,
    total_mmap: int,
    root_paths: list[ProfilePath],
    all_paths: list[ProfilePath],
    total_alloc: int,
    top_n: int,
) -> None:
    non_root_arenas = [row for row in arena_rows if row.name != "<root>"]
    by_subtree_req = sorted(non_root_arenas, key=lambda row: row.subtree_reclaim_req, reverse=True)[:top_n]
    by_direct_mmap = sorted(non_root_arenas, key=lambda row: row.direct_reclaim_mmap, reverse=True)[:top_n]
    top_roots = sorted(root_paths, key=lambda row: row.bytes, reverse=True)[:top_n]
    top_paths = sorted(all_paths, key=lambda row: row.bytes, reverse=True)[:top_n]

    lines = [
        "# Selfhost Memory Ranking",
        "",
        "This combines arena reclaim estimates with sampled allocation call paths.",
        "",
        "## Summary",
        "",
        f"- root requested bytes: `{total_req}` ({fmt_mb(total_req)} MiB)",
        f"- root mmap bytes: `{total_mmap}` ({fmt_mb(total_mmap)} MiB)",
        f"- sampled profile alloc bytes: `{total_alloc}` ({fmt_mb(total_alloc)} MiB)",
        "",
        "## Top Reclaimable Arena Subtrees",
        "",
        "| arena | subtree reclaim req | % root req | subtree reclaim mmap | enters | allocs |",
        "| --- | ---: | ---: | ---: | ---: | ---: |",
    ]
    for row in by_subtree_req:
        lines.append(
            f"| `{row.name}` | `{row.subtree_reclaim_req}` ({fmt_mb(row.subtree_reclaim_req)} MiB) | `{fmt_pct(row.subtree_reclaim_req, total_req)}` | "
            f"`{row.subtree_reclaim_mmap}` ({fmt_mb(row.subtree_reclaim_mmap)} MiB) | `{row.enters}` | `{row.allocs}` |"
        )

    lines.extend(
        [
            "",
            "## Top Direct Reclaimable Mmap Scopes",
            "",
            "| arena | direct reclaim mmap | % root mmap | direct reclaim req |",
            "| --- | ---: | ---: | ---: |",
        ]
    )
    for row in by_direct_mmap:
        lines.append(
            f"| `{row.name}` | `{row.direct_reclaim_mmap}` ({fmt_mb(row.direct_reclaim_mmap)} MiB) | `{fmt_pct(row.direct_reclaim_mmap, total_mmap)}` | "
            f"`{row.direct_reclaim_req}` ({fmt_mb(row.direct_reclaim_req)} MiB) |"
        )

    lines.extend(
        [
            "",
            "## Top Profile Allocation Roots",
            "",
            "| root | sampled bytes | % sampled alloc | calls |",
            "| --- | ---: | ---: | ---: |",
        ]
    )
    for row in top_roots:
        lines.append(
            f"| `{row.path}` | `{row.bytes}` ({fmt_mb(row.bytes)} MiB) | `{fmt_pct(row.bytes, total_alloc)}` | `{row.calls}` |"
        )

    lines.extend(
        [
            "",
            "## Top Profile Allocation Call Paths",
            "",
            "| path | sampled bytes | % sampled alloc | calls |",
            "| --- | ---: | ---: | ---: |",
        ]
    )
    for row in top_paths:
        lines.append(
            f"| `{row.path}` | `{row.bytes}` ({fmt_mb(row.bytes)} MiB) | `{fmt_pct(row.bytes, total_alloc)}` | `{row.calls}` |"
        )

    out.write_text("\n".join(lines) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--arena-tsv", type=Path, required=True)
    parser.add_argument("--profile-report", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--top", type=int, default=15)
    args = parser.parse_args()

    arena_rows, total_req, total_mmap = parse_arena_tsv(args.arena_tsv)
    root_paths, all_paths, total_alloc = parse_profile_paths(args.profile_report)
    write_report(args.out, arena_rows, total_req, total_mmap, root_paths, all_paths, total_alloc, args.top)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
