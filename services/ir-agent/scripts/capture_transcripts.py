#!/usr/bin/env python3
"""Capture replayable demo transcripts for every bundled incident fixture.

Runs each fixture through the real graph (real Anthropic API calls) with a run
accountant attached and writes, per incident, the exact ordered SSE event
sequence — including the trailing ``summary`` event with real token/cost/latency
numbers — to a static JSON file the frontend replays for its free public demo.

Requires a real key (this is the only path that incurs cost):

    IR_ANTHROPIC_API_KEY=sk-... python services/ir-agent/scripts/capture_transcripts.py

Writes to ``frontend/public/ir-agent/transcripts/`` by default (override with
``--out``) plus an ``index.json`` manifest, and prints a measured-outcomes table.
The frontend fetches these as static assets so the replay demo needs no backend.
"""

from __future__ import annotations

import argparse
import json
import sys
from datetime import UTC, datetime
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[3]
# Make `app`/`shared` importable without requiring PYTHONPATH to be set by hand.
sys.path.insert(0, str(REPO_ROOT / "services"))

DEFAULT_OUT = REPO_ROOT / "frontend" / "public" / "ir-agent" / "transcripts"


def _summary_of(transcript: dict) -> dict:
    return next(e["data"] for e in transcript["events"] if e["event"] == "summary")


def _print_row(transcript: dict) -> None:
    s = _summary_of(transcript)
    totals = s["totals"]
    comp = s["comparison"]
    print(
        f"  {transcript['incident_id']:<16} "
        f"${totals['cost_usd']:<7.4f} "
        f"{totals['total_tokens']:>7,} tok  "
        f"{totals['seconds']:>6.1f}s  "
        f"{comp['savings_factor']:>5.2f}x vs Opus  "
        f"attempts={s['investigate_attempts']}  tools={s['tool_calls']}"
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--out",
        type=Path,
        default=DEFAULT_OUT,
        help="Output directory for transcript JSON (default: frontend lib dir).",
    )
    parser.add_argument(
        "--incident",
        action="append",
        dest="incidents",
        help="Limit to specific incident id(s); repeatable. Default: all fixtures.",
    )
    args = parser.parse_args()

    from app import fixtures_store
    from app.main import ROLE_MODELS, _build_graph_app
    from app.transcript_capture import build_transcript

    graph_app = _build_graph_app()  # validates settings (requires the API key)
    incident_ids = args.incidents or fixtures_store.list_incident_ids()

    out_dir: Path = args.out
    out_dir.mkdir(parents=True, exist_ok=True)

    manifest = []
    print(f"Capturing {len(incident_ids)} transcript(s) -> {out_dir}")
    for incident_id in incident_ids:
        incident = fixtures_store.load_incident(incident_id)
        captured_at = datetime.now(UTC).isoformat()
        transcript = build_transcript(
            graph_app,
            incident_id=incident_id,
            title=incident.title,
            role_models=ROLE_MODELS,
            captured_at=captured_at,
        )
        filename = f"{incident_id}.json"
        (out_dir / filename).write_text(json.dumps(transcript, indent=2) + "\n")
        manifest.append(
            {
                "incident_id": incident_id,
                "title": incident.title,
                "source": incident.source,
                "file": filename,
            }
        )
        _print_row(transcript)

    (out_dir / "index.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(f"\nWrote {len(manifest)} transcript(s) + index.json")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
