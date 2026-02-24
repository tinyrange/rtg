#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.10"
# dependencies = [
#   "matplotlib>=3.8",
# ]
# ///

from __future__ import annotations

import argparse
import json
import math
import os
import re
import sys
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib import error, request

import matplotlib.dates as mdates
import matplotlib.pyplot as plt


GITHUB_API_BASE = "https://api.github.com"
DATA_MARKER_RE = re.compile(
    r"<!--\s*compiler-size-data:(\{.*?\})\s*-->", re.DOTALL
)
COMMIT_LINE_RE = re.compile(r"Commit:\s*`([0-9a-f]{7,40})`")


@dataclass
class SizeRecord:
    timestamp: datetime
    commit: str | None
    sizes: dict[str, dict[str, int | None]]
    targets: list[str]
    configs: list[dict[str, str]]


def parse_iso8601(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def github_get_json(url: str, token: str | None) -> Any:
    headers = {
        "Accept": "application/vnd.github+json",
        "X-GitHub-Api-Version": "2022-11-28",
        "User-Agent": "rtg-size-plot-script",
    }
    if token:
        headers["Authorization"] = f"Bearer {token}"

    req = request.Request(url, headers=headers, method="GET")
    try:
        with request.urlopen(req) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(
            f"GitHub API request failed ({exc.code}) for {url}\n{body}"
        ) from exc


def fetch_issue_comments(
    repo: str, issue_number: int, token: str | None
) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    issue = github_get_json(
        f"{GITHUB_API_BASE}/repos/{repo}/issues/{issue_number}",
        token,
    )

    comments: list[dict[str, Any]] = []
    page = 1
    while True:
        batch = github_get_json(
            (
                f"{GITHUB_API_BASE}/repos/{repo}/issues/"
                f"{issue_number}/comments?per_page=100&page={page}"
            ),
            token,
        )
        if not batch:
            break
        comments.extend(batch)
        if len(batch) < 100:
            break
        page += 1

    return issue, comments


def extract_records_from_text(text: str, created_at: str) -> list[SizeRecord]:
    records: list[SizeRecord] = []
    fallback_time = parse_iso8601(created_at)
    commit_from_text_match = COMMIT_LINE_RE.search(text)
    commit_from_text = commit_from_text_match.group(1) if commit_from_text_match else None

    for match in DATA_MARKER_RE.finditer(text):
        raw_json = match.group(1)
        payload = json.loads(raw_json)
        ts = parse_iso8601(payload.get("generated_at", created_at))
        targets = payload.get("targets", [])
        configs = payload.get("configs", [])
        sizes = payload.get("sizes", {})
        commit = payload.get("commit") or commit_from_text

        if not isinstance(targets, list) or not isinstance(configs, list) or not isinstance(sizes, dict):
            continue

        records.append(
            SizeRecord(
                timestamp=ts if ts else fallback_time,
                commit=commit,
                sizes=sizes,
                targets=targets,
                configs=configs,
            )
        )

    return records


def collect_records(repo: str, issue_number: int, token: str | None) -> list[SizeRecord]:
    issue, comments = fetch_issue_comments(repo, issue_number, token)
    records: list[SizeRecord] = []

    issue_body = issue.get("body") or ""
    if issue_body:
        records.extend(extract_records_from_text(issue_body, issue["created_at"]))

    for comment in comments:
        body = comment.get("body") or ""
        if "compiler-size-data:" not in body:
            continue
        records.extend(extract_records_from_text(body, comment["created_at"]))

    records.sort(key=lambda r: (r.timestamp, r.commit or ""))
    return records


def ordered_unique(items: list[str]) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []
    for item in items:
        if item in seen:
            continue
        seen.add(item)
        out.append(item)
    return out


def plot_records(
    records: list[SizeRecord],
    output_path: Path,
    repo: str,
    issue_number: int,
    selected_configs: set[str] | None,
    selected_targets: set[str] | None,
    y_axis: str,
) -> None:
    if not records:
        raise RuntimeError("No compiler-size records found in issue comments.")

    all_targets = ordered_unique([t for r in records for t in r.targets])
    config_name: dict[str, str] = {}
    config_order: list[str] = []
    for record in records:
        for cfg in record.configs:
            cfg_id = cfg.get("id")
            cfg_name = cfg.get("name")
            if not cfg_id or not cfg_name:
                continue
            if cfg_id not in config_name:
                config_order.append(cfg_id)
            config_name[cfg_id] = cfg_name

    if selected_targets is not None:
        all_targets = [t for t in all_targets if t in selected_targets]
    config_ids = config_order
    if selected_configs is not None:
        config_ids = [cfg for cfg in config_ids if cfg in selected_configs]

    if not all_targets:
        raise RuntimeError("No targets left after filtering.")
    if not config_ids:
        raise RuntimeError("No configs left after filtering.")

    nplots = len(config_ids)
    ncols = 2 if nplots > 1 else 1
    nrows = math.ceil(nplots / ncols)
    fig, axes = plt.subplots(
        nrows,
        ncols,
        figsize=(14, max(3.2 * nrows, 4.6)),
        sharex=True,
    )
    if isinstance(axes, plt.Axes):
        axes_list = [axes]
    else:
        axes_list = list(axes.flat)

    cmap = plt.get_cmap("tab10")
    colors = {target: cmap(i % 10) for i, target in enumerate(all_targets)}

    plotted_targets: set[str] = set()
    for i, cfg_id in enumerate(config_ids):
        ax = axes_list[i]
        has_series = False
        for target in all_targets:
            baseline: int | None = None
            xs: list[datetime] = []
            ys: list[float] = []

            for record in records:
                target_sizes = record.sizes.get(target)
                if not isinstance(target_sizes, dict):
                    continue
                value = target_sizes.get(cfg_id)
                if value is None:
                    continue
                if baseline is None:
                    baseline = value
                if baseline == 0:
                    continue

                if y_axis == "percent":
                    ys.append((value - baseline) * 100.0 / baseline)
                else:
                    ys.append(float(value - baseline))
                xs.append(record.timestamp)

            if not xs:
                continue

            ax.plot(
                xs,
                ys,
                marker="o",
                markersize=2.5,
                linewidth=1.3,
                color=colors[target],
                label=target,
            )
            has_series = True
            plotted_targets.add(target)

        ax.axhline(0, color="#888888", linewidth=0.9, linestyle="--")
        ax.grid(True, alpha=0.25, linewidth=0.8)
        ax.set_title(config_name.get(cfg_id, cfg_id), fontsize=10)
        if not has_series:
            ax.text(
                0.5,
                0.5,
                "No matching data",
                ha="center",
                va="center",
                transform=ax.transAxes,
                fontsize=9,
            )

        locator = mdates.AutoDateLocator(minticks=4, maxticks=8)
        formatter = mdates.ConciseDateFormatter(locator)
        ax.xaxis.set_major_locator(locator)
        ax.xaxis.set_major_formatter(formatter)

    for ax in axes_list[nplots:]:
        ax.set_visible(False)

    ylabel = "Change vs first sample (%)" if y_axis == "percent" else "Change vs first sample (bytes)"
    for idx, ax in enumerate(axes_list[:nplots]):
        if idx % ncols == 0:
            ax.set_ylabel(ylabel)
    for ax in axes_list[max(0, nplots - ncols):nplots]:
        ax.set_xlabel("Time")

    if plotted_targets:
        legend_targets = [t for t in all_targets if t in plotted_targets]
        handles = [plt.Line2D([0], [0], color=colors[t], linewidth=1.8) for t in legend_targets]
        fig.legend(
            handles,
            legend_targets,
            loc="lower center",
            ncol=min(4, len(legend_targets)),
            frameon=False,
            bbox_to_anchor=(0.5, 0.01),
            fontsize=9,
        )

    time_start = records[0].timestamp.astimezone(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")
    time_end = records[-1].timestamp.astimezone(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")
    fig.suptitle(
        (
            f"Compiler Size Trends (relative baseline) - {repo} issue #{issue_number}\n"
            f"{len(records)} records from {time_start} to {time_end}"
        ),
        fontsize=11,
    )
    fig.tight_layout(rect=[0.02, 0.06, 0.98, 0.93])

    output_path.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(output_path, dpi=170)
    plt.close(fig)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description=(
            "Fetch compiler-size tracking data from a GitHub issue and plot "
            "target/config trends relative to each series' first sample."
        )
    )
    parser.add_argument("--repo", default="tinyrange/rtg", help="GitHub repo in owner/name form.")
    parser.add_argument("--issue", type=int, default=2, help="Issue number containing size reports.")
    parser.add_argument(
        "--output",
        default="build/compiler-size-relative.png",
        help="Output image path.",
    )
    parser.add_argument(
        "--y-axis",
        choices=("percent", "bytes"),
        default="percent",
        help="Plot relative deltas as percent or bytes.",
    )
    parser.add_argument(
        "--config",
        action="append",
        help="Limit plot to one or more config ids (repeatable).",
    )
    parser.add_argument(
        "--target",
        action="append",
        help="Limit plot to one or more targets (repeatable).",
    )
    parser.add_argument(
        "--show",
        action="store_true",
        help="Open an interactive plot window in addition to writing the file.",
    )
    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    token = os.getenv("GITHUB_TOKEN")

    records = collect_records(args.repo, args.issue, token)
    plot_records(
        records=records,
        output_path=Path(args.output),
        repo=args.repo,
        issue_number=args.issue,
        selected_configs=set(args.config) if args.config else None,
        selected_targets=set(args.target) if args.target else None,
        y_axis=args.y_axis,
    )

    print(f"Wrote plot to {args.output}")
    print(f"Records plotted: {len(records)}")

    if args.show:
        plt.figure()
        img = plt.imread(args.output)
        plt.imshow(img)
        plt.axis("off")
        plt.tight_layout()
        plt.show()

    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except KeyboardInterrupt:
        print("Interrupted.", file=sys.stderr)
        raise SystemExit(130)
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        raise SystemExit(1)
