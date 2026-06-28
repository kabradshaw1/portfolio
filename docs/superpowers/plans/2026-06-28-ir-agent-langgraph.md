# IR Agent — LangGraph Multi-Agent Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `services/ir-agent`, a FastAPI service whose `/investigate` endpoint runs a LangGraph multi-agent pipeline (triage → investigate → validate → report) over a synthetic security incident, streaming results via SSE.

**Architecture:** A LangGraph `StateGraph` threads a typed `IRState` through four role-separated nodes, each backed by its own `ChatAnthropic` model tier. The investigator runs a tool loop over bundled fixtures; the validator adversarially checks findings are grounded in retrieved evidence and (under a retry cap) routes back to the investigator before the reporter produces a structured IR report. Output is validated twice: Pydantic-enforced structured output per node, plus the validator's grounding check.

**Tech Stack:** Python 3.11, FastAPI, `langgraph`, `langchain-core`, `langchain-anthropic`, Pydantic / pydantic-settings, sse-starlette, Prometheus, structlog, pytest / pytest-asyncio.

## Global Constraints

- Python version: `3.11` (Dockerfile `FROM python:3.11-slim`, matching other services).
- Reuse the shared package modules: `shared.auth`, `shared.host_validation`, `shared.logging`, `shared.tracing` (installed via the `shared/` package, as in `services/debug`).
- Tests run with `PYTHONPATH=services pytest services/ir-agent/tests/ -v` and **must not** make real API calls — inject fake models. The only live-API test is gated behind `RUN_LIVE_LLM=1` and skipped otherwise.
- Model ids (exact): triage `claude-haiku-4-5`, investigate `claude-opus-4-8`, validate `claude-opus-4-8`, report `claude-sonnet-4-6`.
- Run `make preflight-python` (ruff lint + format + pytest) and `make preflight-security` before committing Python changes. Pre-commit runs ruff; if it auto-fixes, re-stage and re-commit.
- Service name everywhere: `ir-agent` (package dir `app`, image `ir-agent`, metrics `SERVICE = "ir-agent"`).
- Do not push; commit locally only (per repo convention).

---

## File Structure

```
services/ir-agent/
  Dockerfile                  # Task 13
  requirements.txt            # Task 1
  app/
    __init__.py               # Task 1
    models.py                 # Task 1  — Pydantic contracts
    fixtures_store.py         # Task 2  — load synthetic incidents/evidence
    tools.py                  # Task 3  — LangChain @tool evidence tools
    config.py                 # Task 4  — pydantic-settings, per-role models
    state.py                  # Task 4  — IRState TypedDict
    roles.py                  # Task 4  — role → ChatAnthropic builder
    prompts.py                # Task 4  — per-role system prompts
    nodes/
      __init__.py             # Task 5
      triage.py               # Task 5
      investigate.py          # Task 6
      validate.py             # Task 7
      report.py               # Task 8
    graph.py                  # Task 9  — StateGraph wiring + bounded loop
    metrics.py                # Task 10
    main.py                   # Task 11 — FastAPI app + SSE
  fixtures/
    phishing_credential_theft.json   # Task 2
    malware_beacon_c2.json           # Task 2
    insider_data_exfil.json          # Task 2
  tests/
    __init__.py               # Task 1
    conftest.py               # Task 4  — fake models + rate-limit disable
    test_models.py            # Task 1
    test_fixtures_store.py    # Task 2
    test_tools.py             # Task 3
    test_config.py            # Task 4
    test_roles.py             # Task 4
    test_triage.py            # Task 5
    test_investigate.py       # Task 6
    test_validate.py          # Task 7
    test_report.py            # Task 8
    test_graph.py             # Task 9
    test_metrics.py           # Task 10
    test_main.py              # Task 11
    test_live_smoke.py        # Task 12
docs/adr/ir-agent/            # Task 14
```

---

### Task 1: Pydantic contracts + service scaffold

**Files:**
- Create: `services/ir-agent/app/__init__.py` (empty)
- Create: `services/ir-agent/tests/__init__.py` (empty)
- Create: `services/ir-agent/requirements.txt`
- Create: `services/ir-agent/app/models.py`
- Test: `services/ir-agent/tests/test_models.py`

**Interfaces:**
- Produces: Pydantic models `Observable`, `Incident`, `TriageResult`, `EvidenceItem`, `Findings`, `ValidationVerdict`, `IRReport` (imported by every later task).

- [ ] **Step 1: Create the empty package/test init files and requirements.txt**

`services/ir-agent/app/__init__.py` and `services/ir-agent/tests/__init__.py` are empty files.

`services/ir-agent/requirements.txt`:

```
fastapi==0.135.3
uvicorn[standard]==0.44.0
langgraph==0.6.7
langchain-core==0.3.78
langchain-anthropic==0.3.22
anthropic>=0.30
pydantic-settings==2.3.0
sse-starlette==2.1.0
pytest==9.0.3
pytest-asyncio==1.3.0
pytest-cov==7.1.0
httpx==0.28.1
prometheus-fastapi-instrumentator==7.1.0
slowapi==0.1.9
pyjwt==2.13.0

# Structured logging + tracing
structlog==25.5.0
opentelemetry-api==1.41.0
opentelemetry-sdk==1.41.0
opentelemetry-instrumentation-fastapi==0.62b0
opentelemetry-instrumentation-httpx==0.62b0
opentelemetry-exporter-otlp-proto-grpc==1.41.0
```

- [ ] **Step 2: Write the failing test**

`services/ir-agent/tests/test_models.py`:

```python
import pytest
from pydantic import ValidationError

from app.models import (
    EvidenceItem,
    Findings,
    Incident,
    IRReport,
    Observable,
    TriageResult,
    ValidationVerdict,
)


def test_incident_round_trips():
    inc = Incident(
        id="INC-001",
        source="email-gw",
        title="Suspicious login after phishing email",
        raw={"user": "jdoe"},
        observables=[Observable(kind="user", value="jdoe")],
    )
    assert inc.id == "INC-001"
    assert inc.observables[0].kind == "user"


def test_triage_rejects_bad_severity():
    with pytest.raises(ValidationError):
        TriageResult(
            severity="catastrophic",  # not in the Literal
            category="phishing",
            confidence=0.9,
            rationale="x",
        )


def test_triage_rejects_out_of_range_confidence():
    with pytest.raises(ValidationError):
        TriageResult(
            severity="high",
            category="phishing",
            confidence=1.5,
            rationale="x",
        )


def test_findings_and_verdict_and_report_construct():
    ev = EvidenceItem(id="search_alerts-0", source_tool="search_alerts",
                      query="jdoe", content="alert text")
    f = Findings(summary="s", hypothesis="h", evidence_refs=["search_alerts-0"],
                 iocs=["1.2.3.4"], affected_assets=["host-1"])
    v = ValidationVerdict(grounded=True, unsupported_claims=[], gaps=[],
                          needs_more_investigation=False)
    r = IRReport(executive_summary="e", timeline=["t1"], severity="high",
                 iocs=["1.2.3.4"], mitre_attack=["T1566"],
                 recommended_containment=["isolate host-1"], confidence=0.8)
    assert ev.id in f.evidence_refs
    assert v.grounded is True
    assert r.severity == "high"
```

- [ ] **Step 3: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_models.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'app.models'`.

- [ ] **Step 4: Write minimal implementation**

`services/ir-agent/app/models.py`:

```python
"""Pydantic data contracts for the IR agent graph.

Every node emits one of these validated models — never free text — so the
graph state is type-checked end to end.
"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, Field

Severity = Literal["low", "medium", "high", "critical"]
Category = Literal[
    "phishing", "malware", "lateral-movement", "data-exfil", "other"
]


class Observable(BaseModel):
    kind: Literal["ip", "hash", "domain", "user", "host", "email"]
    value: str


class Incident(BaseModel):
    id: str
    source: Literal["EDR", "SIEM", "email-gw"]
    title: str
    raw: dict = Field(default_factory=dict)
    observables: list[Observable] = Field(default_factory=list)


class TriageResult(BaseModel):
    severity: Severity
    category: Category
    confidence: float = Field(ge=0.0, le=1.0)
    rationale: str


class EvidenceItem(BaseModel):
    id: str
    source_tool: str
    query: str
    content: str


class Findings(BaseModel):
    summary: str
    hypothesis: str
    evidence_refs: list[str] = Field(default_factory=list)
    iocs: list[str] = Field(default_factory=list)
    affected_assets: list[str] = Field(default_factory=list)


class ValidationVerdict(BaseModel):
    grounded: bool
    unsupported_claims: list[str] = Field(default_factory=list)
    gaps: list[str] = Field(default_factory=list)
    needs_more_investigation: bool


class IRReport(BaseModel):
    executive_summary: str
    timeline: list[str] = Field(default_factory=list)
    severity: Severity
    iocs: list[str] = Field(default_factory=list)
    mitre_attack: list[str] = Field(default_factory=list)
    recommended_containment: list[str] = Field(default_factory=list)
    confidence: float = Field(ge=0.0, le=1.0)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_models.py -v`
Expected: PASS (4 tests).

- [ ] **Step 6: Commit**

```bash
git add services/ir-agent/app/__init__.py services/ir-agent/tests/__init__.py \
  services/ir-agent/requirements.txt services/ir-agent/app/models.py \
  services/ir-agent/tests/test_models.py
git commit -m "feat(ir-agent): pydantic contracts and service scaffold"
```

---

### Task 2: Synthetic incident fixtures + loader

**Files:**
- Create: `services/ir-agent/fixtures/phishing_credential_theft.json`
- Create: `services/ir-agent/fixtures/malware_beacon_c2.json`
- Create: `services/ir-agent/fixtures/insider_data_exfil.json`
- Create: `services/ir-agent/app/fixtures_store.py`
- Test: `services/ir-agent/tests/test_fixtures_store.py`

**Interfaces:**
- Consumes: `Incident` from `app.models`.
- Produces:
  - `list_incident_ids() -> list[str]`
  - `load_incident(incident_id: str) -> Incident`
  - `search_alerts_raw(incident_id, query) -> list[dict]`
  - `get_logs_raw(incident_id, selector) -> list[str]`
  - `lookup_ioc_raw(incident_id, indicator) -> dict`
  - `get_asset_context_raw(incident_id, selector) -> dict`
  - Module constant `FIXTURES_DIR: Path`.

- [ ] **Step 1: Create one full fixture and two stubs**

`services/ir-agent/fixtures/phishing_credential_theft.json`:

```json
{
  "incident": {
    "id": "INC-PHISH-001",
    "source": "email-gw",
    "title": "Credential phishing email followed by anomalous login",
    "raw": {"recipient": "jdoe@corp.example", "sender": "it-support@corp-secure.example"},
    "observables": [
      {"kind": "user", "value": "jdoe"},
      {"kind": "domain", "value": "corp-secure.example"},
      {"kind": "ip", "value": "203.0.113.42"},
      {"kind": "host", "value": "WS-JDOE-01"}
    ]
  },
  "alerts": [
    {"id": "A1", "user": "jdoe", "text": "Email gateway flagged credential-harvest URL hxxps://corp-secure.example/login"},
    {"id": "A2", "user": "jdoe", "text": "Successful Okta login for jdoe from 203.0.113.42 (Bucharest, RO) 3m after email click"}
  ],
  "logs": {
    "jdoe": [
      "2026-06-28T09:14Z okta auth success user=jdoe ip=203.0.113.42 geo=RO",
      "2026-06-28T09:15Z okta mfa-bypass user=jdoe method=legacy-token"
    ],
    "WS-JDOE-01": [
      "2026-06-28T09:20Z edr process powershell.exe parent=outlook.exe"
    ]
  },
  "ioc": {
    "203.0.113.42": {"reputation": "malicious", "categories": ["credential-theft", "tor-exit"]},
    "corp-secure.example": {"reputation": "malicious", "categories": ["phishing"]}
  },
  "assets": {
    "jdoe": {"criticality": "high", "owner": "Finance", "role": "AP clerk"},
    "WS-JDOE-01": {"criticality": "medium", "owner": "jdoe", "os": "Windows 11"}
  }
}
```

`services/ir-agent/fixtures/malware_beacon_c2.json`:

```json
{
  "incident": {
    "id": "INC-MAL-002",
    "source": "EDR",
    "title": "Endpoint beaconing to suspected C2",
    "raw": {"host": "WS-ACCT-07"},
    "observables": [
      {"kind": "host", "value": "WS-ACCT-07"},
      {"kind": "ip", "value": "198.51.100.7"},
      {"kind": "hash", "value": "ab12cd34ef56"}
    ]
  },
  "alerts": [
    {"id": "A1", "host": "WS-ACCT-07", "text": "EDR detected periodic 60s beacon to 198.51.100.7"},
    {"id": "A2", "host": "WS-ACCT-07", "text": "Unsigned binary svch0st.exe hash ab12cd34ef56 spawned from temp"}
  ],
  "logs": {
    "WS-ACCT-07": [
      "2026-06-28T11:00Z edr netconn dst=198.51.100.7:443 proc=svch0st.exe",
      "2026-06-28T11:01Z edr netconn dst=198.51.100.7:443 proc=svch0st.exe"
    ]
  },
  "ioc": {
    "198.51.100.7": {"reputation": "malicious", "categories": ["c2", "cobalt-strike"]},
    "ab12cd34ef56": {"reputation": "malicious", "categories": ["trojan"]}
  },
  "assets": {
    "WS-ACCT-07": {"criticality": "high", "owner": "Accounting", "os": "Windows 11"}
  }
}
```

`services/ir-agent/fixtures/insider_data_exfil.json`:

```json
{
  "incident": {
    "id": "INC-EXFIL-003",
    "source": "SIEM",
    "title": "Large outbound transfer to personal cloud storage",
    "raw": {"user": "msmith"},
    "observables": [
      {"kind": "user", "value": "msmith"},
      {"kind": "domain", "value": "personal-drive.example"},
      {"kind": "host", "value": "WS-MSMITH-03"}
    ]
  },
  "alerts": [
    {"id": "A1", "user": "msmith", "text": "DLP: 4.2GB uploaded to personal-drive.example outside policy"},
    {"id": "A2", "user": "msmith", "text": "Access to 'M&A-2026' share spiked 20x in 48h before resignation notice"}
  ],
  "logs": {
    "msmith": [
      "2026-06-27T18:40Z proxy upload bytes=4200000000 dst=personal-drive.example",
      "2026-06-27T18:10Z fileshare read share=M&A-2026 files=312"
    ]
  },
  "ioc": {
    "personal-drive.example": {"reputation": "suspicious", "categories": ["personal-storage"]}
  },
  "assets": {
    "msmith": {"criticality": "medium", "owner": "Corp-Dev", "role": "analyst", "status": "resigning"},
    "WS-MSMITH-03": {"criticality": "medium", "owner": "msmith", "os": "macOS"}
  }
}
```

- [ ] **Step 2: Write the failing test**

`services/ir-agent/tests/test_fixtures_store.py`:

```python
import pytest

from app.fixtures_store import (
    get_asset_context_raw,
    get_logs_raw,
    list_incident_ids,
    load_incident,
    lookup_ioc_raw,
    search_alerts_raw,
)
from app.models import Incident


def test_lists_three_incidents():
    ids = list_incident_ids()
    assert set(ids) == {"INC-PHISH-001", "INC-MAL-002", "INC-EXFIL-003"}


def test_loads_incident_as_model():
    inc = load_incident("INC-PHISH-001")
    assert isinstance(inc, Incident)
    assert inc.source == "email-gw"


def test_load_unknown_incident_raises():
    with pytest.raises(KeyError):
        load_incident("INC-NOPE-999")


def test_search_alerts_filters_by_query():
    hits = search_alerts_raw("INC-PHISH-001", "login")
    assert any("login" in h["text"].lower() for h in hits)


def test_lookup_ioc_returns_reputation():
    rep = lookup_ioc_raw("INC-MAL-002", "198.51.100.7")
    assert rep["reputation"] == "malicious"


def test_get_logs_and_asset_context():
    logs = get_logs_raw("INC-EXFIL-003", "msmith")
    assert any("upload" in line for line in logs)
    asset = get_asset_context_raw("INC-EXFIL-003", "msmith")
    assert asset["status"] == "resigning"
```

- [ ] **Step 3: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_fixtures_store.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'app.fixtures_store'`.

- [ ] **Step 4: Write minimal implementation**

`services/ir-agent/app/fixtures_store.py`:

```python
"""Load synthetic incident scenarios from bundled JSON fixtures.

The fixtures are the deterministic evidence corpus the investigator's tools
read from, so the whole graph is reproducible without external services.
"""

from __future__ import annotations

import json
from functools import lru_cache
from pathlib import Path

from app.models import Incident

FIXTURES_DIR = Path(__file__).resolve().parent.parent / "fixtures"


@lru_cache(maxsize=1)
def _load_all() -> dict[str, dict]:
    """Map incident id -> full fixture dict."""
    store: dict[str, dict] = {}
    for path in sorted(FIXTURES_DIR.glob("*.json")):
        data = json.loads(path.read_text())
        store[data["incident"]["id"]] = data
    return store


def _fixture(incident_id: str) -> dict:
    store = _load_all()
    if incident_id not in store:
        raise KeyError(f"Unknown incident: {incident_id}")
    return store[incident_id]


def list_incident_ids() -> list[str]:
    return sorted(_load_all().keys())


def load_incident(incident_id: str) -> Incident:
    return Incident.model_validate(_fixture(incident_id)["incident"])


def search_alerts_raw(incident_id: str, query: str) -> list[dict]:
    alerts = _fixture(incident_id).get("alerts", [])
    q = query.lower().strip()
    if not q:
        return alerts
    return [a for a in alerts if q in json.dumps(a).lower()]


def get_logs_raw(incident_id: str, selector: str) -> list[str]:
    logs = _fixture(incident_id).get("logs", {})
    if selector in logs:
        return logs[selector]
    out: list[str] = []
    for lines in logs.values():
        out.extend(line for line in lines if selector.lower() in line.lower())
    return out


def lookup_ioc_raw(incident_id: str, indicator: str) -> dict:
    ioc = _fixture(incident_id).get("ioc", {})
    return ioc.get(indicator, {"reputation": "unknown", "categories": []})


def get_asset_context_raw(incident_id: str, selector: str) -> dict:
    assets = _fixture(incident_id).get("assets", {})
    return assets.get(selector, {})
```

- [ ] **Step 5: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_fixtures_store.py -v`
Expected: PASS (6 tests).

- [ ] **Step 6: Commit**

```bash
git add services/ir-agent/fixtures/ services/ir-agent/app/fixtures_store.py \
  services/ir-agent/tests/test_fixtures_store.py
git commit -m "feat(ir-agent): synthetic incident fixtures and loader"
```

---

### Task 3: LangChain evidence tools

**Files:**
- Create: `services/ir-agent/app/tools.py`
- Test: `services/ir-agent/tests/test_tools.py`

**Interfaces:**
- Consumes: the `*_raw` functions from `app.fixtures_store`.
- Produces:
  - `build_tools(incident_id: str) -> list[BaseTool]` — four `@tool`-decorated tools bound to one incident.
  - Tool names (stable): `search_alerts`, `get_logs`, `lookup_ioc`, `get_asset_context`. Each takes a single string arg and returns a string.

- [ ] **Step 1: Write the failing test**

`services/ir-agent/tests/test_tools.py`:

```python
from app.tools import build_tools


def _by_name(tools):
    return {t.name: t for t in tools}


def test_build_tools_returns_four_named_tools():
    tools = _by_name(build_tools("INC-PHISH-001"))
    assert set(tools) == {"search_alerts", "get_logs", "lookup_ioc", "get_asset_context"}


def test_search_alerts_tool_finds_login_alert():
    tools = _by_name(build_tools("INC-PHISH-001"))
    out = tools["search_alerts"].invoke({"query": "login"})
    assert "login" in out.lower()


def test_lookup_ioc_tool_reports_malicious():
    tools = _by_name(build_tools("INC-MAL-002"))
    out = tools["lookup_ioc"].invoke({"indicator": "198.51.100.7"})
    assert "malicious" in out.lower()


def test_get_logs_and_asset_tools():
    tools = _by_name(build_tools("INC-EXFIL-003"))
    logs = tools["get_logs"].invoke({"selector": "msmith"})
    assert "upload" in logs.lower()
    asset = tools["get_asset_context"].invoke({"selector": "msmith"})
    assert "resigning" in asset.lower()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_tools.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'app.tools'`.

- [ ] **Step 3: Write minimal implementation**

`services/ir-agent/app/tools.py`:

```python
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_tools.py -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add services/ir-agent/app/tools.py services/ir-agent/tests/test_tools.py
git commit -m "feat(ir-agent): fixture-backed langchain evidence tools"
```

---

### Task 4: Config, state, prompts, roles + fake-model test harness

**Files:**
- Create: `services/ir-agent/app/config.py`
- Create: `services/ir-agent/app/state.py`
- Create: `services/ir-agent/app/prompts.py`
- Create: `services/ir-agent/app/roles.py`
- Create: `services/ir-agent/tests/conftest.py`
- Test: `services/ir-agent/tests/test_config.py`
- Test: `services/ir-agent/tests/test_roles.py`

**Interfaces:**
- Consumes: `app.models`, `app.tools`.
- Produces:
  - `settings` (a `Settings` instance) with attrs: `anthropic_api_key`, `triage_model`, `investigate_model`, `validate_model`, `report_model`, `max_investigate_attempts: int`, `max_tool_steps: int`, `request_timeout_seconds: int`, `allowed_origins: str`, `jwt_secret: str`. Method `validate() -> None`.
  - `state.IRState` (TypedDict) with keys: `incident: Incident`, `triage`, `evidence: list[EvidenceItem]`, `findings`, `verdict`, `report`, `investigate_attempts: int`.
  - `prompts.TRIAGE_SYSTEM`, `INVESTIGATE_SYSTEM`, `VALIDATE_SYSTEM`, `REPORT_SYSTEM` (str).
  - `roles.Role` (`Literal["triage","investigate","validate","report"]`), `roles.model_for(role, *, builder=ChatAnthropic) -> BaseChatModel`.
  - conftest fixtures: `fake_structured(payload)` factory and `FakeChatModel` class (see Step 5) — reused by Tasks 5–9.

- [ ] **Step 1: Write the failing tests**

`services/ir-agent/tests/test_config.py`:

```python
from app.config import Settings


def test_default_models_are_tiered():
    s = Settings(anthropic_api_key="sk-test")
    assert s.triage_model == "claude-haiku-4-5"
    assert s.investigate_model == "claude-opus-4-8"
    assert s.validate_model == "claude-opus-4-8"
    assert s.report_model == "claude-sonnet-4-6"


def test_validate_requires_api_key():
    s = Settings(anthropic_api_key="")
    try:
        s.validate()
        raised = False
    except ValueError:
        raised = True
    assert raised


def test_loop_bounds_have_defaults():
    s = Settings(anthropic_api_key="sk-test")
    assert s.max_investigate_attempts == 2
    assert s.max_tool_steps == 6
```

`services/ir-agent/tests/test_roles.py`:

```python
from app.roles import model_for


class _RecordingBuilder:
    def __init__(self):
        self.calls = []

    def __call__(self, **kwargs):
        self.calls.append(kwargs)
        return kwargs  # stand-in for a chat model


def test_model_for_uses_correct_model_per_role():
    b = _RecordingBuilder()
    triage = model_for("triage", builder=b)
    report = model_for("report", builder=b)
    assert triage["model"] == "claude-haiku-4-5"
    assert report["model"] == "claude-sonnet-4-6"


def test_model_for_passes_api_key():
    b = _RecordingBuilder()
    model_for("investigate", builder=b)
    assert "api_key" in b.calls[0]
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_config.py services/ir-agent/tests/test_roles.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'app.config'`.

- [ ] **Step 3: Write config, state, prompts, roles**

`services/ir-agent/app/config.py`:

```python
from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    anthropic_api_key: str = ""

    triage_model: str = "claude-haiku-4-5"
    investigate_model: str = "claude-opus-4-8"
    validate_model: str = "claude-opus-4-8"
    report_model: str = "claude-sonnet-4-6"

    max_investigate_attempts: int = 2
    max_tool_steps: int = 6
    request_timeout_seconds: int = 120

    allowed_origins: str = "https://kylebradshaw.dev"
    jwt_secret: str = ""

    model_config = {"env_prefix": "IR_"}

    def validate(self) -> None:
        """Fail fast if the Anthropic key is missing."""
        if not self.anthropic_api_key:
            raise ValueError("IR_ANTHROPIC_API_KEY is required")


settings = Settings()
```

> Note: `settings.validate()` is called in `main.py` (Task 11), not at import, so tests can import `app.config` without a key set.

`services/ir-agent/app/state.py`:

```python
from typing import TypedDict

from app.models import (
    EvidenceItem,
    Findings,
    Incident,
    IRReport,
    TriageResult,
    ValidationVerdict,
)


class IRState(TypedDict, total=False):
    incident: Incident
    triage: TriageResult | None
    evidence: list[EvidenceItem]
    findings: Findings | None
    verdict: ValidationVerdict | None
    report: IRReport | None
    investigate_attempts: int
```

`services/ir-agent/app/prompts.py`:

```python
"""Per-role system prompts. Each prompt defines one agent's job only —
role separation lives here."""

TRIAGE_SYSTEM = (
    "You are a SOC triage analyst. Given a security incident, classify its "
    "severity (low/medium/high/critical), category, and your confidence. "
    "Base the classification only on the incident as presented. Be concise."
)

INVESTIGATE_SYSTEM = (
    "You are an incident-response investigator. Use the provided tools to "
    "gather evidence about the incident before forming a hypothesis. Call "
    "tools to search alerts, pull logs, check IOC reputation, and get asset "
    "context. Every claim in your findings MUST be supported by evidence you "
    "retrieved; cite evidence by its id. Do not speculate beyond the evidence."
)

VALIDATE_SYSTEM = (
    "You are an adversarial reviewer. You are given an investigator's findings "
    "and the exact list of evidence items they had access to. Determine whether "
    "every claim is grounded in that evidence. List any unsupported claims and "
    "any gaps that warrant more investigation. Be strict: if a cited evidence "
    "id does not exist, or a claim has no supporting evidence, it is not grounded."
)

REPORT_SYSTEM = (
    "You are an IR report writer. Produce a clear incident report from the "
    "validated findings: executive summary, timeline, severity, IOCs, MITRE "
    "ATT&CK technique ids, and recommended containment actions."
)
```

`services/ir-agent/app/roles.py`:

```python
"""Map a role to its configured chat model.

Keeping model selection in one place is the per-role tiering decision: cheap
models for classification, strong models for reasoning and validation.
"""

from __future__ import annotations

from typing import Literal

from langchain_anthropic import ChatAnthropic

from app.config import settings

Role = Literal["triage", "investigate", "validate", "report"]

_MODEL_BY_ROLE: dict[Role, str] = {
    "triage": settings.triage_model,
    "investigate": settings.investigate_model,
    "validate": settings.validate_model,
    "report": settings.report_model,
}


def model_for(role: Role, *, builder=ChatAnthropic):
    """Build the chat model for a role. `builder` is injectable for tests."""
    return builder(
        model=_MODEL_BY_ROLE[role],
        api_key=settings.anthropic_api_key,
        timeout=settings.request_timeout_seconds,
        max_retries=2,
    )
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_config.py services/ir-agent/tests/test_roles.py -v`
Expected: PASS (5 tests).

- [ ] **Step 5: Write the shared fake-model conftest (used by Tasks 5–11)**

`services/ir-agent/tests/conftest.py`:

```python
"""Test doubles so node/graph tests never call the real API.

FakeChatModel mimics the small slice of the LangChain chat-model interface the
nodes use: `.with_structured_output(schema)` and `.bind_tools(tools)`, both
returning an object with `.invoke(messages)`.
"""

import pytest
from langchain_core.messages import AIMessage


class _StructuredRunnable:
    def __init__(self, payload):
        self._payload = payload

    def invoke(self, _messages):
        return self._payload


class _ToolRunnable:
    def __init__(self, scripted):
        # scripted: list of AIMessage to return on successive invokes
        self._scripted = list(scripted)

    def invoke(self, _messages):
        return self._scripted.pop(0) if self._scripted else AIMessage(content="done")


class FakeChatModel:
    """Returns canned structured output and a scripted tool-call sequence."""

    def __init__(self, *, structured=None, tool_script=None):
        self._structured = structured
        self._tool_script = tool_script or []

    def with_structured_output(self, _schema):
        return _StructuredRunnable(self._structured)

    def bind_tools(self, _tools):
        return _ToolRunnable(self._tool_script)


@pytest.fixture
def make_fake_model():
    def _factory(*, structured=None, tool_script=None):
        return FakeChatModel(structured=structured, tool_script=tool_script)

    return _factory


@pytest.fixture(autouse=True)
def _disable_rate_limiting():
    """Disable rate limiting when main.py is imported (mirrors debug)."""
    try:
        from app.main import limiter
    except Exception:
        yield
        return
    limiter.enabled = False
    yield
    limiter.enabled = True
```

- [ ] **Step 6: Run the full suite so far**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/ -v`
Expected: PASS (all tests from Tasks 1–4; conftest import does not break collection).

- [ ] **Step 7: Commit**

```bash
git add services/ir-agent/app/config.py services/ir-agent/app/state.py \
  services/ir-agent/app/prompts.py services/ir-agent/app/roles.py \
  services/ir-agent/tests/conftest.py services/ir-agent/tests/test_config.py \
  services/ir-agent/tests/test_roles.py
git commit -m "feat(ir-agent): config, state, prompts, role tiering, fake-model harness"
```

---

### Task 5: Triage node

**Files:**
- Create: `services/ir-agent/app/nodes/__init__.py` (empty)
- Create: `services/ir-agent/app/nodes/triage.py`
- Test: `services/ir-agent/tests/test_triage.py`

**Interfaces:**
- Consumes: `IRState`, `TriageResult`, `prompts.TRIAGE_SYSTEM`. A model exposing `.with_structured_output(schema).invoke(messages)`.
- Produces: `make_triage_node(model) -> Callable[[IRState], dict]`. The node returns `{"triage": TriageResult}`.

- [ ] **Step 1: Write the failing test**

`services/ir-agent/tests/test_triage.py`:

```python
from app.models import Incident, TriageResult
from app.nodes.triage import make_triage_node


def test_triage_node_sets_triage(make_fake_model):
    expected = TriageResult(severity="high", category="phishing",
                            confidence=0.9, rationale="login from RO after phish")
    model = make_fake_model(structured=expected)
    node = make_triage_node(model)
    state = {"incident": Incident(id="INC-PHISH-001", source="email-gw",
                                  title="t")}
    out = node(state)
    assert out["triage"] == expected
```

- [ ] **Step 2: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_triage.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'app.nodes.triage'`.

- [ ] **Step 3: Write minimal implementation**

`services/ir-agent/app/nodes/__init__.py` is empty.

`services/ir-agent/app/nodes/triage.py`:

```python
"""Triage node: classify the incident with structured output."""

from __future__ import annotations

from collections.abc import Callable

from langchain_core.messages import HumanMessage, SystemMessage

from app.models import TriageResult
from app.prompts import TRIAGE_SYSTEM
from app.state import IRState


def make_triage_node(model) -> Callable[[IRState], dict]:
    structured = model.with_structured_output(TriageResult)

    def triage_node(state: IRState) -> dict:
        incident = state["incident"]
        messages = [
            SystemMessage(content=TRIAGE_SYSTEM),
            HumanMessage(content=incident.model_dump_json()),
        ]
        result: TriageResult = structured.invoke(messages)
        return {"triage": result}

    return triage_node
```

- [ ] **Step 4: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_triage.py -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/ir-agent/app/nodes/__init__.py services/ir-agent/app/nodes/triage.py \
  services/ir-agent/tests/test_triage.py
git commit -m "feat(ir-agent): triage node with structured output"
```

---

### Task 6: Investigate node (tool loop + findings)

**Files:**
- Create: `services/ir-agent/app/nodes/investigate.py`
- Test: `services/ir-agent/tests/test_investigate.py`

**Interfaces:**
- Consumes: `IRState`, `Findings`, `EvidenceItem`, `prompts.INVESTIGATE_SYSTEM`, `app.tools.build_tools`. A model exposing `.bind_tools(tools).invoke(messages)` (returns an `AIMessage`; `.tool_calls` is a list of `{"name","args","id"}`) and `.with_structured_output(Findings).invoke(messages)`.
- Produces: `make_investigate_node(model, *, max_tool_steps) -> Callable[[IRState], dict]`. The node returns `{"findings": Findings, "evidence": list[EvidenceItem], "investigate_attempts": int}`. Evidence accumulates across retries (prior evidence is preserved and extended). On each invocation `investigate_attempts` is incremented by 1.

- [ ] **Step 1: Write the failing test**

`services/ir-agent/tests/test_investigate.py`:

```python
from langchain_core.messages import AIMessage

from app.models import Findings, Incident
from app.nodes.investigate import make_investigate_node


def _tool_call(name, args, call_id):
    return {"name": name, "args": args, "id": call_id, "type": "tool_call"}


def test_investigate_runs_tools_then_produces_findings(make_fake_model):
    # First model turn: ask to search alerts; second turn: no tool calls (stop).
    tool_script = [
        AIMessage(content="", tool_calls=[
            _tool_call("search_alerts", {"query": "login"}, "c1")
        ]),
        AIMessage(content="done"),
    ]
    findings = Findings(summary="creds phished", hypothesis="account takeover",
                        evidence_refs=["search_alerts-0"], iocs=["203.0.113.42"],
                        affected_assets=["jdoe"])
    model = make_fake_model(structured=findings, tool_script=tool_script)
    node = make_investigate_node(model, max_tool_steps=4)

    state = {"incident": Incident(id="INC-PHISH-001", source="email-gw", title="t")}
    out = node(state)

    assert out["findings"] == findings
    assert out["investigate_attempts"] == 1
    # one tool call ran -> one evidence item, with a stable id
    assert len(out["evidence"]) == 1
    assert out["evidence"][0].id == "search_alerts-0"
    assert out["evidence"][0].source_tool == "search_alerts"


def test_investigate_increments_attempts_and_keeps_prior_evidence(make_fake_model):
    findings = Findings(summary="s", hypothesis="h")
    model = make_fake_model(structured=findings, tool_script=[AIMessage(content="done")])
    node = make_investigate_node(model, max_tool_steps=4)

    from app.models import EvidenceItem
    prior = [EvidenceItem(id="search_alerts-0", source_tool="search_alerts",
                          query="x", content="old")]
    state = {"incident": Incident(id="INC-PHISH-001", source="email-gw", title="t"),
             "evidence": prior, "investigate_attempts": 1}
    out = node(state)
    assert out["investigate_attempts"] == 2
    assert out["evidence"][0].id == "search_alerts-0"  # prior preserved
```

- [ ] **Step 2: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_investigate.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'app.nodes.investigate'`.

- [ ] **Step 3: Write minimal implementation**

`services/ir-agent/app/nodes/investigate.py`:

```python
"""Investigate node: a bounded ReAct tool loop that gathers evidence, then a
structured call that produces grounded findings."""

from __future__ import annotations

from collections.abc import Callable

from langchain_core.messages import HumanMessage, SystemMessage, ToolMessage

from app.models import EvidenceItem, Findings
from app.prompts import INVESTIGATE_SYSTEM
from app.state import IRState
from app.tools import build_tools


def make_investigate_node(model, *, max_tool_steps: int) -> Callable[[IRState], dict]:
    def investigate_node(state: IRState) -> dict:
        incident = state["incident"]
        tools = {t.name: t for t in build_tools(incident.id)}
        bound = model.bind_tools(list(tools.values()))

        evidence: list[EvidenceItem] = list(state.get("evidence") or [])
        counter = len(evidence)

        prompt = f"Incident:\n{incident.model_dump_json()}"
        triage = state.get("triage")
        if triage is not None:
            prompt += f"\n\nTriage:\n{triage.model_dump_json()}"
        verdict = state.get("verdict")
        if verdict is not None and verdict.gaps:
            prompt += "\n\nPrior review found gaps to address:\n" + "\n".join(
                verdict.gaps
            )

        messages = [SystemMessage(content=INVESTIGATE_SYSTEM),
                    HumanMessage(content=prompt)]

        for _ in range(max_tool_steps):
            ai = bound.invoke(messages)
            messages.append(ai)
            tool_calls = getattr(ai, "tool_calls", None) or []
            if not tool_calls:
                break
            for call in tool_calls:
                name = call["name"]
                args = call.get("args", {})
                tool = tools.get(name)
                result = (
                    tool.invoke(args) if tool else f"Unknown tool: {name}"
                )
                eid = f"{name}-{counter}"
                counter += 1
                evidence.append(EvidenceItem(
                    id=eid, source_tool=name,
                    query=", ".join(str(v) for v in args.values()),
                    content=str(result),
                ))
                messages.append(ToolMessage(
                    content=f"[{eid}] {result}", tool_call_id=call["id"],
                ))

        evidence_catalog = "\n".join(f"{e.id}: {e.content}" for e in evidence)
        findings: Findings = model.with_structured_output(Findings).invoke(
            messages
            + [HumanMessage(
                content="Produce findings. Cite evidence by id. "
                f"Available evidence ids:\n{evidence_catalog}"
            )]
        )

        return {
            "findings": findings,
            "evidence": evidence,
            "investigate_attempts": state.get("investigate_attempts", 0) + 1,
        }

    return investigate_node
```

- [ ] **Step 4: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_investigate.py -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add services/ir-agent/app/nodes/investigate.py services/ir-agent/tests/test_investigate.py
git commit -m "feat(ir-agent): investigate node with bounded tool loop"
```

---

### Task 7: Validate node + routing

**Files:**
- Create: `services/ir-agent/app/nodes/validate.py`
- Test: `services/ir-agent/tests/test_validate.py`

**Interfaces:**
- Consumes: `IRState`, `ValidationVerdict`, `prompts.VALIDATE_SYSTEM`. A model exposing `.with_structured_output(ValidationVerdict).invoke(messages)`.
- Produces:
  - `make_validate_node(model) -> Callable[[IRState], dict]` returning `{"verdict": ValidationVerdict}`.
  - `route_after_validate(state: IRState, *, max_attempts: int) -> str` returning `"investigate"` or `"report"`.

- [ ] **Step 1: Write the failing test**

`services/ir-agent/tests/test_validate.py`:

```python
from app.models import Findings, ValidationVerdict
from app.nodes.validate import make_validate_node, route_after_validate


def test_validate_node_sets_verdict(make_fake_model):
    verdict = ValidationVerdict(grounded=True, unsupported_claims=[], gaps=[],
                               needs_more_investigation=False)
    model = make_fake_model(structured=verdict)
    node = make_validate_node(model)
    out = node({"findings": Findings(summary="s", hypothesis="h"), "evidence": []})
    assert out["verdict"] == verdict


def test_route_to_report_when_grounded():
    state = {"verdict": ValidationVerdict(grounded=True, needs_more_investigation=False),
             "investigate_attempts": 1}
    assert route_after_validate(state, max_attempts=2) == "report"


def test_route_back_to_investigate_when_ungrounded_under_cap():
    state = {"verdict": ValidationVerdict(grounded=False, needs_more_investigation=True),
             "investigate_attempts": 1}
    assert route_after_validate(state, max_attempts=2) == "investigate"


def test_route_to_report_when_cap_reached():
    state = {"verdict": ValidationVerdict(grounded=False, needs_more_investigation=True),
             "investigate_attempts": 2}
    assert route_after_validate(state, max_attempts=2) == "report"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_validate.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'app.nodes.validate'`.

- [ ] **Step 3: Write minimal implementation**

`services/ir-agent/app/nodes/validate.py`:

```python
"""Validate node: adversarial grounding check + loop routing."""

from __future__ import annotations

from collections.abc import Callable

from langchain_core.messages import HumanMessage, SystemMessage

from app.models import ValidationVerdict
from app.prompts import VALIDATE_SYSTEM
from app.state import IRState


def make_validate_node(model) -> Callable[[IRState], dict]:
    structured = model.with_structured_output(ValidationVerdict)

    def validate_node(state: IRState) -> dict:
        findings = state["findings"]
        evidence = state.get("evidence") or []
        catalog = "\n".join(f"{e.id}: {e.content}" for e in evidence)
        prompt = (
            f"Findings:\n{findings.model_dump_json()}\n\n"
            f"Evidence available to the investigator:\n{catalog or '(none)'}"
        )
        verdict: ValidationVerdict = structured.invoke([
            SystemMessage(content=VALIDATE_SYSTEM),
            HumanMessage(content=prompt),
        ])
        return {"verdict": verdict}

    return validate_node


def route_after_validate(state: IRState, *, max_attempts: int) -> str:
    """Decide whether to re-investigate or proceed to the report."""
    verdict = state["verdict"]
    attempts = state.get("investigate_attempts", 0)
    needs_more = (not verdict.grounded) or verdict.needs_more_investigation
    if needs_more and attempts < max_attempts:
        return "investigate"
    return "report"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_validate.py -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add services/ir-agent/app/nodes/validate.py services/ir-agent/tests/test_validate.py
git commit -m "feat(ir-agent): validate node and bounded loop routing"
```

---

### Task 8: Report node

**Files:**
- Create: `services/ir-agent/app/nodes/report.py`
- Test: `services/ir-agent/tests/test_report.py`

**Interfaces:**
- Consumes: `IRState`, `IRReport`, `prompts.REPORT_SYSTEM`. A model exposing `.with_structured_output(IRReport).invoke(messages)`.
- Produces: `make_report_node(model) -> Callable[[IRState], dict]` returning `{"report": IRReport}`.

- [ ] **Step 1: Write the failing test**

`services/ir-agent/tests/test_report.py`:

```python
from app.models import Findings, IRReport, TriageResult, ValidationVerdict
from app.nodes.report import make_report_node


def test_report_node_sets_report(make_fake_model):
    report = IRReport(executive_summary="e", timeline=["t"], severity="high",
                      iocs=["203.0.113.42"], mitre_attack=["T1566"],
                      recommended_containment=["disable jdoe"], confidence=0.85)
    model = make_fake_model(structured=report)
    node = make_report_node(model)
    state = {
        "triage": TriageResult(severity="high", category="phishing",
                               confidence=0.9, rationale="r"),
        "findings": Findings(summary="s", hypothesis="h"),
        "verdict": ValidationVerdict(grounded=True, needs_more_investigation=False),
    }
    out = node(state)
    assert out["report"] == report
```

- [ ] **Step 2: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_report.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'app.nodes.report'`.

- [ ] **Step 3: Write minimal implementation**

`services/ir-agent/app/nodes/report.py`:

```python
"""Report node: produce the final structured IR report."""

from __future__ import annotations

from collections.abc import Callable

from langchain_core.messages import HumanMessage, SystemMessage

from app.models import IRReport
from app.prompts import REPORT_SYSTEM
from app.state import IRState


def make_report_node(model) -> Callable[[IRState], dict]:
    structured = model.with_structured_output(IRReport)

    def report_node(state: IRState) -> dict:
        parts = []
        triage = state.get("triage")
        if triage is not None:
            parts.append(f"Triage:\n{triage.model_dump_json()}")
        findings = state.get("findings")
        if findings is not None:
            parts.append(f"Findings:\n{findings.model_dump_json()}")
        verdict = state.get("verdict")
        if verdict is not None:
            parts.append(f"Validation:\n{verdict.model_dump_json()}")
        report: IRReport = structured.invoke([
            SystemMessage(content=REPORT_SYSTEM),
            HumanMessage(content="\n\n".join(parts)),
        ])
        return {"report": report}

    return report_node
```

- [ ] **Step 4: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_report.py -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/ir-agent/app/nodes/report.py services/ir-agent/tests/test_report.py
git commit -m "feat(ir-agent): report node with structured IR report"
```

---

### Task 9: Graph assembly + bounded loop

**Files:**
- Create: `services/ir-agent/app/graph.py`
- Test: `services/ir-agent/tests/test_graph.py`

**Interfaces:**
- Consumes: all four `make_*_node` factories, `route_after_validate`, `IRState`, `settings`.
- Produces: `build_graph(models: dict[str, object], *, max_tool_steps: int, max_attempts: int)` where `models` maps role names to chat models. Returns a compiled LangGraph app with `.invoke(state)` and `.stream(state)`. The compiled graph runs `triage → investigate → validate → (investigate | report) → END`.

- [ ] **Step 1: Write the failing test**

`services/ir-agent/tests/test_graph.py`:

```python
from langchain_core.messages import AIMessage

from app.graph import build_graph
from app.models import (
    Findings,
    Incident,
    IRReport,
    TriageResult,
    ValidationVerdict,
)
from tests.conftest import FakeChatModel


def _models(*, verdicts):
    """Build a role->model map. `verdicts` is a list the validate model
    returns on successive invocations (drives the loop)."""

    class _SequencedValidate:
        def __init__(self, seq):
            self._seq = list(seq)

        def with_structured_output(self, _schema):
            outer = self

            class _R:
                def invoke(self, _m):
                    return outer._seq.pop(0)

            return _R()

    return {
        "triage": FakeChatModel(structured=TriageResult(
            severity="high", category="phishing", confidence=0.9, rationale="r")),
        "investigate": FakeChatModel(
            structured=Findings(summary="s", hypothesis="h",
                                evidence_refs=["search_alerts-0"]),
            tool_script=[AIMessage(content="", tool_calls=[
                {"name": "search_alerts", "args": {"query": "login"},
                 "id": "c1", "type": "tool_call"}]),
                AIMessage(content="done")]),
        "validate": _SequencedValidate(verdicts),
        "report": FakeChatModel(structured=IRReport(
            executive_summary="e", severity="high", confidence=0.8)),
    }


def _start_state():
    return {"incident": Incident(id="INC-PHISH-001", source="email-gw", title="t"),
            "evidence": [], "investigate_attempts": 0}


def test_happy_path_reaches_report():
    grounded = ValidationVerdict(grounded=True, needs_more_investigation=False)
    app = build_graph(_models(verdicts=[grounded]), max_tool_steps=4, max_attempts=2)
    out = app.invoke(_start_state())
    assert out["report"].severity == "high"
    assert out["investigate_attempts"] == 1


def test_ungrounded_then_grounded_loops_once():
    bad = ValidationVerdict(grounded=False, needs_more_investigation=True,
                            gaps=["no IOC checked"])
    good = ValidationVerdict(grounded=True, needs_more_investigation=False)
    app = build_graph(_models(verdicts=[bad, good]), max_tool_steps=4, max_attempts=2)
    out = app.invoke(_start_state())
    assert out["report"] is not None
    assert out["investigate_attempts"] == 2  # investigated twice


def test_loop_is_bounded():
    bad = ValidationVerdict(grounded=False, needs_more_investigation=True)
    # always ungrounded; cap=2 means investigate runs at most twice, then report
    app = build_graph(_models(verdicts=[bad, bad, bad]),
                      max_tool_steps=4, max_attempts=2)
    out = app.invoke(_start_state())
    assert out["report"] is not None
    assert out["investigate_attempts"] == 2
```

- [ ] **Step 2: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_graph.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'app.graph'`.

- [ ] **Step 3: Write minimal implementation**

`services/ir-agent/app/graph.py`:

```python
"""Assemble the IR investigation graph.

triage -> investigate -> validate -> (investigate | report) -> END
The validate->investigate edge is bounded by max_attempts (enforced in
route_after_validate against investigate_attempts).
"""

from __future__ import annotations

from functools import partial

from langgraph.graph import END, START, StateGraph

from app.nodes.investigate import make_investigate_node
from app.nodes.report import make_report_node
from app.nodes.triage import make_triage_node
from app.nodes.validate import make_validate_node, route_after_validate
from app.state import IRState


def build_graph(models: dict, *, max_tool_steps: int, max_attempts: int):
    graph = StateGraph(IRState)
    graph.add_node("triage", make_triage_node(models["triage"]))
    graph.add_node(
        "investigate",
        make_investigate_node(models["investigate"], max_tool_steps=max_tool_steps),
    )
    graph.add_node("validate", make_validate_node(models["validate"]))
    graph.add_node("report", make_report_node(models["report"]))

    graph.add_edge(START, "triage")
    graph.add_edge("triage", "investigate")
    graph.add_edge("investigate", "validate")
    graph.add_conditional_edges(
        "validate",
        partial(route_after_validate, max_attempts=max_attempts),
        {"investigate": "investigate", "report": "report"},
    )
    graph.add_edge("report", END)
    return graph.compile()
```

- [ ] **Step 4: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_graph.py -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add services/ir-agent/app/graph.py services/ir-agent/tests/test_graph.py
git commit -m "feat(ir-agent): compile langgraph stategraph with bounded loop"
```

---

### Task 10: Prometheus metrics

**Files:**
- Create: `services/ir-agent/app/metrics.py`
- Test: `services/ir-agent/tests/test_metrics.py`

**Interfaces:**
- Produces: `instrumentator` (an `Instrumentator`), and collectors `NODE_DURATION` (Histogram, labels `["node"]`), `TOOL_CALLS` (Counter, labels `["tool"]`), `INVESTIGATE_ATTEMPTS` (Histogram, labels `[]` via `.observe`), `LLM_TOKENS` (Counter, labels `["role","kind"]`). `SERVICE = "ir-agent"`.

- [ ] **Step 1: Write the failing test**

`services/ir-agent/tests/test_metrics.py`:

```python
from app.metrics import (
    INVESTIGATE_ATTEMPTS,
    LLM_TOKENS,
    NODE_DURATION,
    SERVICE,
    TOOL_CALLS,
    instrumentator,
)


def test_service_label():
    assert SERVICE == "ir-agent"


def test_collectors_accept_labels():
    NODE_DURATION.labels(node="triage").observe(0.1)
    TOOL_CALLS.labels(tool="search_alerts").inc()
    INVESTIGATE_ATTEMPTS.observe(1)
    LLM_TOKENS.labels(role="triage", kind="prompt").inc(10)
    assert instrumentator is not None
```

- [ ] **Step 2: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_metrics.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'app.metrics'`.

- [ ] **Step 3: Write minimal implementation**

`services/ir-agent/app/metrics.py`:

```python
"""Prometheus metrics for the IR agent service."""

from prometheus_client import Counter, Histogram
from prometheus_fastapi_instrumentator import Instrumentator

SERVICE = "ir-agent"

instrumentator = Instrumentator(
    should_group_status_codes=False,
    excluded_handlers=["/health", "/metrics"],
)

NODE_DURATION = Histogram(
    "ir_node_duration_seconds",
    "Wall-clock time per graph node",
    ["node"],
    buckets=(0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0, 300.0),
)

TOOL_CALLS = Counter(
    "ir_tool_calls_total",
    "Total evidence-tool calls by the investigator",
    ["tool"],
)

INVESTIGATE_ATTEMPTS = Histogram(
    "ir_investigate_attempts",
    "Number of investigate passes per incident (validator loop)",
    buckets=(1, 2, 3, 4, 5),
)

LLM_TOKENS = Counter(
    "ir_llm_tokens_total",
    "Tokens used per role",
    ["role", "kind"],
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_metrics.py -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/ir-agent/app/metrics.py services/ir-agent/tests/test_metrics.py
git commit -m "feat(ir-agent): prometheus metrics collectors"
```

---

### Task 11: FastAPI app + SSE streaming

**Files:**
- Create: `services/ir-agent/app/main.py`
- Test: `services/ir-agent/tests/test_main.py`

**Interfaces:**
- Consumes: `build_graph`, `roles.model_for`, `fixtures_store`, `settings`, `metrics.instrumentator`, shared middleware/auth.
- Produces: FastAPI `app`; `limiter`; module-level `_build_graph_app()` that constructs the compiled graph from real role models (called lazily). Endpoints: `GET /health`, `POST /investigate` (auth-required, SSE). `/investigate` body: `{"incident_id": str}` (must be a known fixture) — streams events `triage`, `evidence`, `findings`, `verdict`, `report`, `done`.

- [ ] **Step 1: Write the failing test**

`services/ir-agent/tests/test_main.py`:

```python
import app.main as main_module
from app.models import IRReport, TriageResult
from fastapi.testclient import TestClient


def test_health_ok(monkeypatch):
    client = TestClient(main_module.app)
    resp = client.get("/health")
    assert resp.status_code in (200, 503)
    assert "status" in resp.json()


def test_investigate_streams_report(monkeypatch):
    # Replace the compiled graph with a fake that streams node outputs.
    class _FakeGraph:
        def stream(self, _state):
            yield {"triage": TriageResult(severity="high", category="phishing",
                                          confidence=0.9, rationale="r")}
            yield {"report": IRReport(executive_summary="e", severity="high",
                                      confidence=0.8)}

    monkeypatch.setattr(main_module, "_graph_app", _FakeGraph())
    # Bypass auth dependency
    main_module.app.dependency_overrides[main_module.require_auth] = lambda: "tester"

    client = TestClient(main_module.app)
    resp = client.post("/investigate", json={"incident_id": "INC-PHISH-001"})
    assert resp.status_code == 200
    body = resp.text
    assert "triage" in body
    assert "report" in body
    main_module.app.dependency_overrides.clear()


def test_investigate_unknown_incident_400(monkeypatch):
    main_module.app.dependency_overrides[main_module.require_auth] = lambda: "tester"
    client = TestClient(main_module.app)
    resp = client.post("/investigate", json={"incident_id": "NOPE"})
    assert resp.status_code == 400
    main_module.app.dependency_overrides.clear()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_main.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'app.main'`.

- [ ] **Step 3: Write minimal implementation**

`services/ir-agent/app/main.py`:

```python
import json

import structlog
from fastapi import Depends, FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field
from shared.auth import create_auth_dependency
from shared.host_validation import HostHeaderValidationMiddleware
from shared.logging import RequestLoggingMiddleware, configure_logging
from shared.tracing import configure_tracing, instrument_app
from slowapi import Limiter
from slowapi.errors import RateLimitExceeded
from slowapi.util import get_remote_address
from sse_starlette.sse import EventSourceResponse
from starlette.requests import Request

from app import fixtures_store
from app.config import settings
from app.graph import build_graph
from app.metrics import INVESTIGATE_ATTEMPTS, instrumentator
from app.roles import model_for

logger = structlog.get_logger()

configure_logging(service_name="ir-agent")
configure_tracing(service_name="ir-agent")

app = FastAPI(title="IR Agent API")

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.allowed_origins.split(","),
    allow_methods=["GET", "POST"],
    allow_headers=["Authorization", "Content-Type"],
)
app.add_middleware(RequestLoggingMiddleware)
app.add_middleware(HostHeaderValidationMiddleware)

instrumentator.instrument(app).expose(app, include_in_schema=False)
instrument_app(app)

limiter = Limiter(key_func=get_remote_address)
app.state.limiter = limiter

require_auth = create_auth_dependency(settings.jwt_secret)

# Lazily-built compiled graph (None until first /investigate or test injection).
_graph_app = None


def _build_graph_app():
    settings.validate()
    models = {role: model_for(role) for role in
              ("triage", "investigate", "validate", "report")}
    return build_graph(
        models,
        max_tool_steps=settings.max_tool_steps,
        max_attempts=settings.max_investigate_attempts,
    )


def _get_graph_app():
    global _graph_app
    if _graph_app is None:
        _graph_app = _build_graph_app()
    return _graph_app


@app.exception_handler(RateLimitExceeded)
async def rate_limit_handler(request: Request, exc: RateLimitExceeded):
    return JSONResponse(status_code=429, content={"detail": "Rate limit exceeded"})


class InvestigateRequest(BaseModel):
    incident_id: str = Field(pattern=r"^[A-Za-z0-9_-]{1,64}$")


@app.get("/health")
async def health():
    api_key_ok = bool(settings.anthropic_api_key)
    fixtures_ok = len(fixtures_store.list_incident_ids()) > 0
    healthy = api_key_ok and fixtures_ok
    return JSONResponse(
        status_code=200 if healthy else 503,
        content={
            "status": "healthy" if healthy else "degraded",
            "anthropic_key": "set" if api_key_ok else "missing",
            "fixtures": "loaded" if fixtures_ok else "missing",
        },
    )


def _serialize(value) -> str:
    return value.model_dump_json() if hasattr(value, "model_dump_json") else json.dumps(value)


@app.post("/investigate")
@limiter.limit("10/minute")
async def investigate(
    request: Request, body: InvestigateRequest, user_id: str = Depends(require_auth)
):
    if body.incident_id not in fixtures_store.list_incident_ids():
        raise HTTPException(status_code=400, detail="Unknown incident_id")

    incident = fixtures_store.load_incident(body.incident_id)
    start_state = {"incident": incident, "evidence": [], "investigate_attempts": 0}
    graph_app = _get_graph_app()

    async def event_generator():
        try:
            for chunk in graph_app.stream(start_state):
                for _node, update in chunk.items():
                    for key in ("triage", "evidence", "findings", "verdict", "report"):
                        if key in update and update[key] is not None:
                            payload = update[key]
                            if key == "evidence":
                                data = json.dumps([e.model_dump() for e in payload])
                            else:
                                data = _serialize(payload)
                            yield {"event": key, "data": data}
                    if "investigate_attempts" in update:
                        INVESTIGATE_ATTEMPTS.observe(update["investigate_attempts"])
            yield {"event": "done", "data": json.dumps({})}
        except Exception as e:  # noqa: BLE001
            logger.error("investigation_error", error=str(e), exc_info=True)
            yield {"event": "error",
                   "data": json.dumps({"detail": "Internal error during investigation."})}
            yield {"event": "done", "data": json.dumps({})}

    return EventSourceResponse(event_generator())
```

> Note: `graph_app.stream` yields, per LangGraph convention, a dict keyed by node name whose value is that node's state update — the generator iterates `chunk.items()` to extract updates. The test injects a `_FakeGraph` whose `stream` yields update dicts directly; `chunk.items()` over `{"triage": ...}` works for both shapes.

- [ ] **Step 4: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_main.py -v`
Expected: PASS (3 tests). (The autouse `_disable_rate_limiting` conftest fixture imports `app.main.limiter` and disables it.)

- [ ] **Step 5: Run the whole suite**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/ -v`
Expected: PASS (all tests; no network calls).

- [ ] **Step 6: Commit**

```bash
git add services/ir-agent/app/main.py services/ir-agent/tests/test_main.py
git commit -m "feat(ir-agent): fastapi app with SSE investigate endpoint"
```

---

### Task 12: Live smoke test (gated, skipped in CI)

**Files:**
- Test: `services/ir-agent/tests/test_live_smoke.py`

**Interfaces:**
- Consumes: `_build_graph_app` from `app.main`, `fixtures_store`.

- [ ] **Step 1: Write the gated test**

`services/ir-agent/tests/test_live_smoke.py`:

```python
"""End-to-end run against the real Anthropic API.

Skipped unless RUN_LIVE_LLM=1 and IR_ANTHROPIC_API_KEY are set. This is the
only test that incurs API cost — run it manually a handful of times.
"""

import os

import pytest

RUN_LIVE = os.getenv("RUN_LIVE_LLM") == "1"

pytestmark = pytest.mark.skipif(
    not RUN_LIVE, reason="set RUN_LIVE_LLM=1 to run the live end-to-end smoke test"
)


def test_full_investigation_against_real_api():
    from app import fixtures_store
    from app.main import _build_graph_app

    graph_app = _build_graph_app()
    incident = fixtures_store.load_incident("INC-PHISH-001")
    out = graph_app.invoke(
        {"incident": incident, "evidence": [], "investigate_attempts": 0}
    )
    assert out["report"] is not None
    assert out["report"].severity in {"low", "medium", "high", "critical"}
    assert out["triage"] is not None
    assert out["investigate_attempts"] >= 1
```

- [ ] **Step 2: Verify it is skipped by default**

Run: `PYTHONPATH=services pytest services/ir-agent/tests/test_live_smoke.py -v`
Expected: 1 skipped.

- [ ] **Step 3: (Manual, optional) Run it live once**

Run: `RUN_LIVE_LLM=1 IR_ANTHROPIC_API_KEY=sk-... PYTHONPATH=services pytest services/ir-agent/tests/test_live_smoke.py -v`
Expected: PASS. Cost ≈ $0.15. Confirm a spend limit is set in the Anthropic console first.

- [ ] **Step 4: Commit**

```bash
git add services/ir-agent/tests/test_live_smoke.py
git commit -m "test(ir-agent): gated live end-to-end smoke test"
```

---

### Task 13: Dockerfile + repo wiring (compose + CI)

**Files:**
- Create: `services/ir-agent/Dockerfile`
- Modify: `docker-compose.yml` (add `ir-agent` service block)
- Modify: `.github/workflows/ci.yml` (backend test matrix, docker build matrix, pip-audit matrix, Hadolint matrix, deploy pull commands)

**Interfaces:** none (infra).

- [ ] **Step 1: Create the Dockerfile**

`services/ir-agent/Dockerfile`:

```dockerfile
FROM python:3.11-slim

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1

WORKDIR /app

# Install shared package first (changes less frequently)
COPY shared/ /shared/
RUN pip install --no-cache-dir /shared

COPY ir-agent/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY shared/ ./shared/
COPY ir-agent/app/ ./app/
COPY ir-agent/fixtures/ ./fixtures/

RUN useradd --create-home appuser
USER appuser

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
```

- [ ] **Step 2: Lint the Dockerfile**

Run: `hadolint services/ir-agent/Dockerfile` (if installed) or `make preflight-security`.
Expected: no errors (mirrors the existing `debug/Dockerfile`, which passes).

- [ ] **Step 3: Add the docker-compose service block**

In `docker-compose.yml`, add after the `debug:` block (match its style; the build `context` is `./services` and the dockerfile path is `ir-agent/Dockerfile`):

```yaml
  ir-agent:
    image: ghcr.io/kabradshaw1/portfolio/ir-agent:latest
    build:
      context: ./services
      dockerfile: ir-agent/Dockerfile
    env_file: .env
    environment:
      - IR_ANTHROPIC_API_KEY=${IR_ANTHROPIC_API_KEY:-}
      - IR_JWT_SECRET=${JWT_SECRET:-}
    extra_hosts:
      - "host.docker.internal:host-gateway"
```

- [ ] **Step 4: Add CI matrix entries**

In `.github/workflows/ci.yml`, add to the **backend test matrix** (after the `rag-triage` entry near line 66):

```yaml
          - service: ir-agent
            paths: services/ir-agent services/shared .github/workflows/ci.yml .github/actions/check-changes/action.yml
```

Add `ir-agent` to the **docker build matrix**, the **pip-audit matrix**, and the **Hadolint Dockerfile matrix** in the same file (mirror how `debug` appears in each). Add an `ir-agent` entry to the **CI deploy pull commands** alongside the other services.

> The exact line numbers for the build / pip-audit / Hadolint / deploy sections vary; grep for `debug` in `.github/workflows/ci.yml` and add a parallel `ir-agent` entry everywhere `debug` appears in a matrix or pull list.

- [ ] **Step 5: Validate compose and CI YAML**

Run: `docker compose -f docker-compose.yml config >/dev/null && echo OK`
Expected: `OK` (compose parses).
Run: `python -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml')); print('ci yaml ok')"`
Expected: `ci yaml ok`.

- [ ] **Step 6: Commit**

```bash
git add services/ir-agent/Dockerfile docker-compose.yml .github/workflows/ci.yml
git commit -m "ci(ir-agent): dockerfile, compose service, CI matrix wiring"
```

---

### Task 14: ADR + env example + preflight

**Files:**
- Create: `docs/adr/ir-agent/` ADR (notebook or markdown, per the writing-adrs skill)
- Modify: `.env.example` (add `IR_ANTHROPIC_API_KEY`)

**Interfaces:** none (docs/config).

- [ ] **Step 1: Add the env var to `.env.example`**

Append to `.env.example`:

```
# IR agent (LangGraph multi-agent incident response)
IR_ANTHROPIC_API_KEY=
```

- [ ] **Step 2: Write the ADR**

Invoke the `writing-adrs` skill to produce a companion ADR under `docs/adr/ir-agent/` covering: the LangGraph topology and why a linear-pipeline-with-validation-loop over a supervisor; per-role model tiering (Haiku/Opus/Sonnet) and its rationale; the two-layer output-validation design (Pydantic structured output + adversarial grounding check); and why the service uses `langchain-anthropic`/`langgraph` directly rather than the legacy `shared/llm` abstraction. Reference the spec at `docs/superpowers/specs/2026-06-28-ir-agent-langgraph-design.md`.

- [ ] **Step 3: Run full preflight**

Run: `make preflight-python`
Expected: ruff lint + format clean; the existing service test suites plus a manual `PYTHONPATH=services pytest services/ir-agent/tests/ -v` all pass.

> If `make preflight-python` does not yet include `ir-agent`, also run `PYTHONPATH=services pytest services/ir-agent/tests/ -v` explicitly and add an `ir-agent` pytest line to the `preflight-python` target in the `Makefile` (mirror the `debug` line at `Makefile:19-20`).

Run: `make preflight-security`
Expected: secret scan + bandit + pip-audit clean.

- [ ] **Step 4: Commit**

```bash
git add docs/adr/ir-agent/ .env.example Makefile
git commit -m "docs(ir-agent): ADR, env example, preflight wiring"
```

---

## Self-Review

**Spec coverage:**
- Multi-agent LangGraph topology → Tasks 5–9. ✓
- Role separation (per-role prompts + models) → Tasks 4 (prompts/roles), 5–8. ✓
- Context injection (structured state, not raw transcript) → Task 4 (state), Tasks 6/8 read structured state. ✓
- Output validation layer 1 (Pydantic structured output) → every node uses `with_structured_output` (Tasks 5–8). ✓
- Output validation layer 2 (grounding check + bounded loop) → Tasks 7, 9. ✓
- Per-role Claude tiering (Haiku/Opus/Sonnet) → Task 4. ✓
- Tools over fixtures, deterministic → Tasks 2, 3. ✓
- FastAPI + SSE surface, /health, /metrics → Tasks 10, 11. ✓
- Config (per-role models, key, loop caps, caching note) → Task 4. ✓ (Prompt-caching is a runtime concern handled by `langchain-anthropic`; no separate task needed for v1.)
- Deterministic, CI-safe tests + gated live smoke → conftest (Task 4), Task 12. ✓
- Dependencies → Task 1. ✓
- Repo integration (compose, CI matrices, ADR, env) → Tasks 13, 14. ✓
- Out-of-scope items (supervisor, parallel, MCP, runbook tool) correctly absent. ✓

**Placeholder scan:** No TBDs; every code step shows complete code. Task 13 Step 4 intentionally points the engineer to grep for `debug` in `ci.yml` because the matrix line numbers drift — this is a concrete, actionable instruction, not a placeholder.

**Type consistency:** `make_triage_node` / `make_investigate_node` / `make_validate_node` / `make_report_node` and `route_after_validate` names are consistent across Tasks 5–9. `build_graph(models, *, max_tool_steps, max_attempts)` signature matches its call in `main.py` (Task 11). `IRState` keys (`incident`, `triage`, `evidence`, `findings`, `verdict`, `report`, `investigate_attempts`) are consistent across state, nodes, graph, and main. Tool names (`search_alerts`, `get_logs`, `lookup_ioc`, `get_asset_context`) are consistent between Tasks 3 and 6. Model ids match the spec and the claude-api catalog.
