#!/usr/bin/env python3

import argparse
from dataclasses import dataclass, field
from pathlib import Path
from typing import Dict, List


@dataclass
class Row:
    id: int
    parent: int
    depth: int
    enters: int
    allocs: int
    req: int
    mmap: int
    retained_req: int
    retained_mmap: int
    name: str
    children: List["Row"] = field(default_factory=list)
    direct_reclaim_req: int = 0
    direct_reclaim_mmap: int = 0
    subtree_reclaim_req: int = 0
    subtree_reclaim_mmap: int = 0


def parse_rows(path: Path) -> Dict[int, Row]:
    rows: Dict[int, Row] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line or line.startswith("arena_report") or line.startswith("note=") or line.startswith("id "):
            continue
        parts = line.split()
        if len(parts) < 11:
            continue
        row = Row(
            id=int(parts[0]),
            parent=int(parts[1]),
            depth=int(parts[2]),
            enters=int(parts[4]),
            allocs=int(parts[5]),
            req=int(parts[6]),
            mmap=int(parts[7]),
            retained_req=int(parts[8]),
            retained_mmap=int(parts[9]),
            name=parts[10],
        )
        row.direct_reclaim_req = max(row.req - row.retained_req, 0)
        row.direct_reclaim_mmap = max(row.mmap - row.retained_mmap, 0)
        rows[row.id] = row
    for row in rows.values():
        parent = rows.get(row.parent)
        if parent is not None:
            parent.children.append(row)
    return rows


def compute_subtree(row: Row) -> None:
    total_req = row.direct_reclaim_req
    total_mmap = row.direct_reclaim_mmap
    for child in row.children:
        compute_subtree(child)
        total_req += child.subtree_reclaim_req
        total_mmap += child.subtree_reclaim_mmap
    row.subtree_reclaim_req = total_req
    row.subtree_reclaim_mmap = total_mmap


def fmt_mb(n: int) -> str:
    return f"{n / (1024 * 1024):.1f}"


def fmt_pct(part: int, total: int) -> str:
    if total <= 0:
        return "0.0%"
    return f"{(100.0 * part) / total:.1f}%"


def write_tsv(path: Path, rows: List[Row]) -> None:
    lines = [
        "\t".join(
            [
                "id",
                "depth",
                "enters",
                "allocs",
                "req_bytes",
                "retained_req_bytes",
                "direct_reclaim_req_bytes",
                "subtree_reclaim_req_bytes",
                "mmap_bytes",
                "retained_mmap_bytes",
                "direct_reclaim_mmap_bytes",
                "subtree_reclaim_mmap_bytes",
                "name",
            ]
        )
    ]
    for row in rows:
        lines.append(
            "\t".join(
                [
                    str(row.id),
                    str(row.depth),
                    str(row.enters),
                    str(row.allocs),
                    str(row.req),
                    str(row.retained_req),
                    str(row.direct_reclaim_req),
                    str(row.subtree_reclaim_req),
                    str(row.mmap),
                    str(row.retained_mmap),
                    str(row.direct_reclaim_mmap),
                    str(row.subtree_reclaim_mmap),
                    row.name,
                ]
            )
        )
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def write_md(path: Path, rows: List[Row], total_req: int, total_mmap: int, top_n: int) -> None:
    non_root = [row for row in rows if row.id != 1]
    by_direct_req = sorted(non_root, key=lambda row: row.direct_reclaim_req, reverse=True)[:top_n]
    by_subtree_req = sorted(non_root, key=lambda row: row.subtree_reclaim_req, reverse=True)[:top_n]
    by_direct_mmap = sorted(non_root, key=lambda row: row.direct_reclaim_mmap, reverse=True)[:top_n]

    lines = [
        "# Arena Gain Estimates",
        "",
        "These are estimates, not exact peak-RSS deltas.",
        "",
        "- `direct_reclaim_*`: bytes allocated in this scope and not retained upward.",
        "- `subtree_reclaim_*`: direct reclaimable bytes plus all reclaimable child scopes.",
        "- `*_mmap_*` is the best proxy for peak RSS reduction in the current allocator.",
        "- `*_req_*` is the best proxy for total allocation pressure removed.",
        "",
        f"- root requested bytes: `{total_req}` (`{fmt_mb(total_req)} MiB`)",
        f"- root mmap bytes: `{total_mmap}` (`{fmt_mb(total_mmap)} MiB`)",
        "",
        "## Top Direct Reclaimable Request Bytes",
        "",
        "| arena | direct reclaim req | % root req | direct reclaim mmap | enters | allocs |",
        "| --- | ---: | ---: | ---: | ---: | ---: |",
    ]
    for row in by_direct_req:
        lines.append(
            f"| `{row.name}` | `{row.direct_reclaim_req}` ({fmt_mb(row.direct_reclaim_req)} MiB) | `{fmt_pct(row.direct_reclaim_req, total_req)}` | "
            f"`{row.direct_reclaim_mmap}` ({fmt_mb(row.direct_reclaim_mmap)} MiB) | `{row.enters}` | `{row.allocs}` |"
        )

    lines.extend(
        [
            "",
            "## Top Subtree Reclaimable Request Bytes",
            "",
            "| arena | subtree reclaim req | % root req | subtree reclaim mmap | direct reclaim req |",
            "| --- | ---: | ---: | ---: | ---: |",
        ]
    )
    for row in by_subtree_req:
        lines.append(
            f"| `{row.name}` | `{row.subtree_reclaim_req}` ({fmt_mb(row.subtree_reclaim_req)} MiB) | `{fmt_pct(row.subtree_reclaim_req, total_req)}` | "
            f"`{row.subtree_reclaim_mmap}` ({fmt_mb(row.subtree_reclaim_mmap)} MiB) | `{row.direct_reclaim_req}` ({fmt_mb(row.direct_reclaim_req)} MiB) |"
        )

    lines.extend(
        [
            "",
            "## Top Direct Reclaimable Mmap Bytes",
            "",
            "| arena | direct reclaim mmap | % root mmap | direct reclaim req | retained req |",
            "| --- | ---: | ---: | ---: | ---: |",
        ]
    )
    for row in by_direct_mmap:
        lines.append(
            f"| `{row.name}` | `{row.direct_reclaim_mmap}` ({fmt_mb(row.direct_reclaim_mmap)} MiB) | `{fmt_pct(row.direct_reclaim_mmap, total_mmap)}` | "
            f"`{row.direct_reclaim_req}` ({fmt_mb(row.direct_reclaim_req)} MiB) | `{row.retained_req}` ({fmt_mb(row.retained_req)} MiB) |"
        )

    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("report", type=Path)
    parser.add_argument("--md-out", type=Path, required=True)
    parser.add_argument("--tsv-out", type=Path, required=True)
    parser.add_argument("--top", type=int, default=20)
    args = parser.parse_args()

    rows_by_id = parse_rows(args.report)
    root = rows_by_id.get(1)
    if root is None:
        raise SystemExit("missing root row")
    compute_subtree(root)
    rows = [rows_by_id[idx] for idx in sorted(rows_by_id)]
    write_tsv(args.tsv_out, rows)
    write_md(args.md_out, rows, root.req, root.mmap, args.top)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
