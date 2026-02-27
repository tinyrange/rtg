#!/usr/bin/env python3

from __future__ import annotations

import argparse
import html
import os
import re
import shutil
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path


SUMMARY_RE = re.compile(r"^records=(\d+)\s+unique=(\d+)\s+total_ns=(\d+)\s*$")
NODE_RE = re.compile(
    r"^(?P<prefix>(?:\|  |   )*)(?:\|- |\\- )(?P<name>.+?) total=(?P<total>\d+)ns(?: calls=(?P<calls>\d+) avg=(?P<avg>\d+)ns)?\s*$"
)


@dataclass
class Node:
    name: str
    total_ns: int
    calls: int | None = None
    children: list["Node"] = field(default_factory=list)


@dataclass
class Frame:
    node: Node
    depth: int
    x: float
    width: float


def parse_report_text(text: str) -> tuple[str | None, int | None, int | None, int | None, Node]:
    profile_path: str | None = None
    records: int | None = None
    unique: int | None = None
    total_ns: int | None = None

    root = Node(name="<root>", total_ns=0)
    stack: list[Node] = [root]

    for raw_line in text.splitlines():
        line = raw_line.rstrip("\n")
        if line.startswith("Profile report for "):
            profile_path = line[len("Profile report for ") :].strip()
            continue

        summary = SUMMARY_RE.match(line)
        if summary:
            records = int(summary.group(1))
            unique = int(summary.group(2))
            total_ns = int(summary.group(3))
            root.total_ns = total_ns
            continue

        match = NODE_RE.match(line)
        if not match:
            continue

        depth = len(match.group("prefix")) // 3
        name = match.group("name")
        node_total = int(match.group("total"))
        node_calls = int(match.group("calls")) if match.group("calls") else None
        node = Node(name=name, total_ns=node_total, calls=node_calls)

        while len(stack) > depth + 1:
            stack.pop()
        parent = stack[-1]
        parent.children.append(node)
        stack.append(node)

    if root.total_ns == 0:
        root.total_ns = sum(child.total_ns for child in root.children)

    return profile_path, records, unique, total_ns, root


def hash_color(name: str) -> str:
    h = 2166136261
    for ch in name:
        h ^= ord(ch)
        h = (h * 16777619) & 0xFFFFFFFF
    hue = h % 360
    sat = 55 + (h >> 8) % 30
    light = 45 + (h >> 16) % 20
    return f"hsl({hue} {sat}% {light}%)"


def collect_frames(
    node: Node,
    depth: int,
    x: float,
    width: float,
    out: list[Frame],
    force_denom: float | None = None,
) -> None:
    out.append(Frame(node=node, depth=depth, x=x, width=width))
    if not node.children or width <= 0:
        return

    child_total = 0
    for child in node.children:
        if child.total_ns > 0:
            child_total += child.total_ns

    denom = force_denom if force_denom is not None else node.total_ns
    if denom <= 0:
        denom = child_total
    elif child_total > denom:
        # Edge totals can over-count in cyclic call graphs; shrink to fit parent.
        denom = child_total
    if denom <= 0:
        return

    cur = x
    for child in node.children:
        child_w = width * (child.total_ns / denom)
        collect_frames(child, depth + 1, cur, child_w, out)
        cur += child_w


def truncate_label(label: str, px_width: float) -> str:
    max_chars = int((px_width - 8) // 7)
    if max_chars <= 0:
        return ""
    if len(label) <= max_chars:
        return label
    if max_chars <= 3:
        return ""
    return label[: max_chars - 3] + "..."


def build_html(
    title: str,
    profile_path: str | None,
    records: int | None,
    unique: int | None,
    root: Node,
    min_percent: float,
) -> str:
    chart_width = 1600.0
    bar_height = 22.0
    bar_gap = 2.0
    top_padding = 10.0

    frames: list[Frame] = []
    render_total = sum(child.total_ns for child in root.children)
    if render_total <= 0:
        render_total = root.total_ns
    if render_total <= 0:
        raise RuntimeError("Profile totals are zero.")
    # Always render a single explicit root row so caller/callee hierarchy stays clear.
    render_root = Node(name="<root>", total_ns=render_total, calls=None, children=root.children)
    collect_frames(render_root, 0, 0.0, chart_width, frames, force_denom=render_total)

    if not frames:
        raise RuntimeError("No profile tree nodes found in report.")

    max_depth = max(frame.depth for frame in frames)
    chart_height = top_padding + (max_depth + 1) * (bar_height + bar_gap) + 10
    min_width = chart_width * (min_percent / 100.0)

    rects: list[str] = []
    for frame in frames:
        if frame.depth > 0 and frame.width < min_width:
            continue
        y = top_padding + frame.depth * (bar_height + bar_gap)
        pct = (frame.node.total_ns * 100.0 / render_total) if render_total > 0 else 0.0
        label = truncate_label(frame.node.name, frame.width)
        safe_name = html.escape(frame.node.name, quote=True)
        safe_label = html.escape(label)
        inferred_parent = frame.depth > 0 and frame.node.calls is None and frame.node.children
        if frame.depth == 0:
            color = "#7f8a98"
        elif inferred_parent:
            color = "#8a6f45"
        else:
            color = hash_color(frame.node.name)
        calls_text = f"{frame.node.calls:,}" if frame.node.calls is not None else "inferred"
        details = (
            f"{frame.node.name}\n"
            f"total={frame.node.total_ns:,}ns ({pct:.2f}%)\n"
            f"calls={calls_text}"
        )
        if inferred_parent:
            details += "\n(no direct samples; parent inferred from edge aggregation)"
        safe_details = html.escape(details, quote=True)
        text = ""
        if safe_label:
            text = (
                f'<text x="{frame.x + 4:.2f}" y="{y + 15:.2f}" class="label">{safe_label}</text>'
            )
        rects.append(
            (
                f'<g class="frame" data-tip="{safe_details}">'
                f'<rect x="{frame.x:.2f}" y="{y:.2f}" width="{frame.width:.2f}" height="{bar_height:.2f}" '
                f'fill="{color}" data-name="{safe_name}"></rect>'
                f"{text}</g>"
            )
        )

    subtitle_bits = [f"tree_total={render_total:,}ns"]
    if root.total_ns > 0 and root.total_ns != render_total:
        subtitle_bits.append(f"summary_total={root.total_ns:,}ns")
    if records is not None:
        subtitle_bits.append(f"records={records:,}")
    if unique is not None:
        subtitle_bits.append(f"unique={unique:,}")
    if profile_path:
        subtitle_bits.append(f"source={profile_path}")

    subtitle = html.escape(" | ".join(subtitle_bits))
    safe_title = html.escape(title)

    return f"""<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{safe_title}</title>
  <style>
    :root {{
      --bg: #101316;
      --panel: #151a1f;
      --fg: #e5e7eb;
      --muted: #93a4b8;
      --grid: #29313a;
    }}
    * {{ box-sizing: border-box; }}
    body {{
      margin: 0;
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      background: linear-gradient(180deg, #0d1013 0%, var(--bg) 65%);
      color: var(--fg);
    }}
    .wrap {{
      padding: 20px;
      max-width: 1800px;
      margin: 0 auto;
    }}
    h1 {{
      margin: 0 0 8px;
      font-size: 20px;
      font-weight: 700;
    }}
    .sub {{
      margin: 0 0 12px;
      color: var(--muted);
      font-size: 13px;
    }}
    .chart {{
      border: 1px solid var(--grid);
      background: var(--panel);
      border-radius: 10px;
      overflow: auto;
      padding: 8px;
    }}
    svg {{
      width: 100%;
      min-width: 900px;
      display: block;
    }}
    .frame rect {{
      stroke: rgba(255, 255, 255, 0.14);
      stroke-width: 0.6;
      cursor: default;
    }}
    .frame:hover rect {{
      stroke: #ffffff;
      stroke-width: 1.1;
      filter: brightness(1.1);
    }}
    .label {{
      fill: rgba(0, 0, 0, 0.78);
      font-size: 12px;
      pointer-events: none;
      user-select: none;
    }}
    #tip {{
      position: fixed;
      display: none;
      white-space: pre;
      z-index: 20;
      background: rgba(5, 8, 12, 0.95);
      border: 1px solid #3b4755;
      color: #f8fafc;
      border-radius: 6px;
      font-size: 12px;
      line-height: 1.35;
      padding: 8px 10px;
      pointer-events: none;
    }}
  </style>
</head>
<body>
  <div class="wrap">
    <h1>{safe_title}</h1>
    <p class="sub">{subtitle}</p>
    <div class="chart">
      <svg viewBox="0 0 {chart_width:.2f} {chart_height:.2f}" role="img" aria-label="Flamechart">
        {''.join(rects)}
      </svg>
    </div>
  </div>
  <div id="tip"></div>
  <script>
    const tip = document.getElementById("tip");
    for (const frame of document.querySelectorAll(".frame")) {{
      frame.addEventListener("mousemove", (ev) => {{
        tip.style.display = "block";
        tip.textContent = frame.dataset.tip || "";
        tip.style.left = (ev.clientX + 14) + "px";
        tip.style.top = (ev.clientY + 14) + "px";
      }});
      frame.addEventListener("mouseleave", () => {{
        tip.style.display = "none";
      }});
    }}
  </script>
</body>
</html>
"""


def generate_report_from_profile(compiler: str, profile_path: str, entries: list[str]) -> str:
    cmd = [compiler, "-profile-report", profile_path]
    cmd.extend(entries)
    proc = subprocess.run(cmd, capture_output=True, text=True)
    if proc.returncode != 0:
        stderr = proc.stderr.strip()
        stdout = proc.stdout.strip()
        details = stderr if stderr else stdout
        if not details:
            details = f"exit status {proc.returncode}"
        raise RuntimeError(f"failed to run {' '.join(cmd)}: {details}")
    return proc.stdout


def open_in_browser(path: Path) -> None:
    resolved = str(path.resolve())
    if sys.platform == "darwin":
        subprocess.run(["open", resolved], check=True)
        return

    if os.name == "nt":
        os.startfile(resolved)  # type: ignore[attr-defined]
        return

    opener = shutil.which("xdg-open")
    if opener is None:
        raise RuntimeError("cannot open browser automatically: xdg-open not found")
    subprocess.run([opener, resolved], check=True)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Convert RTG profile data into a standalone flamechart HTML file. "
            "Input can be existing -profile-report text, or a raw .profile file via --profile."
        )
    )
    parser.add_argument(
        "report",
        nargs="?",
        help="Path to text report output, or '-' for stdin.",
    )
    parser.add_argument(
        "--profile",
        help="Path to raw RTG profile binary file (uses compiler -profile-report).",
    )
    parser.add_argument(
        "--compiler",
        default="./build/rtg",
        help="Compiler path to run when --profile is used (default: ./build/rtg).",
    )
    parser.add_argument(
        "--entry",
        action="append",
        default=[],
        help="Entry file/directory passed to -profile-report for hash->name mapping; can be repeated.",
    )
    parser.add_argument(
        "--report-out",
        help="Optional path to save generated text report when --profile is used.",
    )
    parser.add_argument(
        "-o",
        "--output",
        required=True,
        help="Output HTML file path.",
    )
    parser.add_argument(
        "--title",
        default="RTG Profile Flamechart",
        help="Chart title.",
    )
    parser.add_argument(
        "--min-percent",
        type=float,
        default=0.0,
        help="Hide non-root frames narrower than this percent of full width (default: 0.0).",
    )
    parser.add_argument(
        "--open",
        action="store_true",
        help="Open generated HTML in the default browser.",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if args.min_percent < 0:
        print("--min-percent must be >= 0", file=sys.stderr)
        return 2

    if args.profile:
        entries = list(args.entry)
        if not entries:
            entries = ["./std/compiler/"]
        try:
            text = generate_report_from_profile(args.compiler, args.profile, entries)
        except RuntimeError as err:
            print(str(err), file=sys.stderr)
            return 1
        if args.report_out:
            Path(args.report_out).write_text(text, encoding="utf-8")
    elif args.report == "-":
        text = sys.stdin.read()
    elif args.report:
        text = Path(args.report).read_text(encoding="utf-8")
    else:
        print("provide either a report path (or '-') or --profile <path>", file=sys.stderr)
        return 2

    profile_path, records, unique, _, root = parse_report_text(text)
    if not root.children or root.total_ns <= 0:
        print("No profile frames found in input.", file=sys.stderr)
        return 1

    html_out = build_html(
        title=args.title,
        profile_path=profile_path,
        records=records,
        unique=unique,
        root=root,
        min_percent=args.min_percent,
    )

    out_path = Path(args.output)
    out_path.write_text(html_out, encoding="utf-8")
    print(f"Wrote flamechart HTML to {out_path}")
    if args.report_out:
        print(f"Wrote profile report text to {args.report_out}")
    if args.open:
        try:
            open_in_browser(out_path)
            print(f"Opened {out_path} in default browser")
        except Exception as err:
            print(f"Failed to open browser: {err}", file=sys.stderr)
            return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
