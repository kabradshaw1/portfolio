"""Evidence-gathering tools for the investigator agent.

Each tool is bound to a single incident and reads from the bundled fixtures,
so tool calls are deterministic. Tools return plain strings (the model reads
them); the investigate node wraps results into EvidenceItem records.
"""

from __future__ import annotations

import json

from langchain_core.tools import BaseTool, tool

from app import fixtures_store


def build_tools(incident_id: str) -> list[BaseTool]:
    """Return the four evidence tools, closed over one incident id."""

    @tool
    def search_alerts(query: str) -> str:
        """Search this incident's security alerts for a keyword or observable."""
        hits = fixtures_store.search_alerts_raw(incident_id, query)
        return json.dumps(hits) if hits else "No matching alerts."

    @tool
    def get_logs(selector: str) -> str:
        """Fetch log lines for a host, user, or matching substring."""
        lines = fixtures_store.get_logs_raw(incident_id, selector)
        return "\n".join(lines) if lines else "No matching logs."

    @tool
    def lookup_ioc(indicator: str) -> str:
        """Look up threat-intel reputation for an IP, hash, or domain."""
        return json.dumps(fixtures_store.lookup_ioc_raw(incident_id, indicator))

    @tool
    def get_asset_context(selector: str) -> str:
        """Get asset/identity context (criticality, owner) for a host or user."""
        ctx = fixtures_store.get_asset_context_raw(incident_id, selector)
        return json.dumps(ctx) if ctx else "No asset context found."

    return [search_alerts, get_logs, lookup_ioc, get_asset_context]
