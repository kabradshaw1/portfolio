# Pip-Audit Advisory Backlog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove all 10 `--ignore-vuln` flags from the pip-audit CI step by dropping unused `langchain-community`, upgrading `langchain-text-splitters` and `pytest`, and verifying a clean audit.

**Architecture:** Drop `langchain-community` (unused) from all 3 Python services. Upgrade `langchain-text-splitters` from 0.2.4 to latest (≥1.1.2 for GHSA-fv5p fix). Upgrade `pytest` from 8.4.2 to ≥9.0.3. Remove all `--ignore-vuln` lines. Verify with pip-audit.

**Tech Stack:** Python 3.11, pip-audit, langchain-text-splitters, pytest 9.x

**Spec:** `docs/superpowers/specs/2026-04-16-pip-audit-backlog-design.md`
**Issue:** #62

---

### Task 1: Upgrade `langchain-text-splitters` and drop `langchain-community` in ingestion service

**Files:**
- Modify: `services/ingestion/requirements.txt`

- [ ] **Step 1: Edit requirements.txt**

Remove `langchain-community==0.2.19` (line 6). Change `langchain-text-splitters==0.2.4` (line 5) to the latest version. Use context7 or PyPI to find the current latest `langchain-text-splitters` version (must be ≥1.1.2 to fix GHSA-fv5p-p927-qmxr).

```diff
- langchain-text-splitters==0.2.4
- langchain-community==0.2.19
+ langchain-text-splitters==<latest>
```

- [ ] **Step 2: Verify imports still work**

Run: `cd services/ingestion && pip install -r requirements.txt && python -c "from langchain_text_splitters import RecursiveCharacterTextSplitter; print('OK')"`

Expected: `OK` — the import path is unchanged across versions.

- [ ] **Step 3: Run tests**

Run: `pytest services/ingestion/tests/ -v`

Expected: All tests pass. Key test: `test_chunker.py` exercises `RecursiveCharacterTextSplitter` directly.

- [ ] **Step 4: Commit**

```bash
git add services/ingestion/requirements.txt
git commit -m "fix(deps): drop langchain-community, upgrade text-splitters in ingestion"
```

---

### Task 2: Upgrade `langchain-text-splitters` and drop `langchain-community` in debug service

**Files:**
- Modify: `services/debug/requirements.txt`

- [ ] **Step 1: Edit requirements.txt**

Remove `langchain-community==0.2.19` (line 4). Change `langchain-text-splitters==0.2.4` (line 3) to the same version used in Task 1.

```diff
- langchain-text-splitters==0.2.4
- langchain-community==0.2.19
+ langchain-text-splitters==<same version as Task 1>
```

- [ ] **Step 2: Verify imports still work**

Run: `cd services/debug && pip install -r requirements.txt && python -c "from langchain_text_splitters import Language, RecursiveCharacterTextSplitter; print('OK')"`

Expected: `OK` — both `Language` and `RecursiveCharacterTextSplitter` are available. The `Language.PYTHON` enum and `from_language()` factory are stable across versions.

- [ ] **Step 3: Run tests**

Run: `pytest services/debug/tests/ -v`

Expected: All tests pass. Key tests: `test_indexer.py` exercises `chunk_code_files` (which uses `Language.PYTHON` splitting) and `index_project`.

- [ ] **Step 4: Commit**

```bash
git add services/debug/requirements.txt
git commit -m "fix(deps): drop langchain-community, upgrade text-splitters in debug"
```

---

### Task 3: Drop `langchain-community` from chat service

**Files:**
- Modify: `services/chat/requirements.txt`

- [ ] **Step 1: Edit requirements.txt**

Remove `langchain-community==0.2.19` (line 3). No replacement needed — the chat service has zero langchain imports.

```diff
- langchain-community==0.2.19
```

- [ ] **Step 2: Run tests**

Run: `pytest services/chat/tests/ -v`

Expected: All tests pass. Chat service doesn't use langchain at all.

- [ ] **Step 3: Commit**

```bash
git add services/chat/requirements.txt
git commit -m "fix(deps): drop unused langchain-community from chat"
```

---

### Task 4: Upgrade pytest across all three services

**Files:**
- Modify: `services/chat/requirements.txt`
- Modify: `services/ingestion/requirements.txt`
- Modify: `services/debug/requirements.txt`

- [ ] **Step 1: Check latest pytest 9.x and plugin compatibility**

Use PyPI/context7 to find:
- Latest `pytest` 9.x version (must be ≥9.0.3 for CVE-2025-71176)
- Whether `pytest-asyncio==0.26.0` is compatible with pytest 9.x (if not, find compatible version)
- Whether `pytest-cov==5.0.0` (chat) and `pytest-cov==7.1.0` (ingestion/debug) are compatible

- [ ] **Step 2: Update pytest in all three requirements.txt**

```diff
# In all three files:
- pytest==8.4.2
+ pytest==<latest 9.x>
```

Also bump `pytest-asyncio` and `pytest-cov` if needed for compatibility.

- [ ] **Step 3: Run all tests**

Run: `make preflight-python`

Expected: All tests pass across all three services.

- [ ] **Step 4: Commit**

```bash
git add services/chat/requirements.txt services/ingestion/requirements.txt services/debug/requirements.txt
git commit -m "fix(deps): upgrade pytest to 9.x (CVE-2025-71176)"
```

---

### Task 5: Verify dependency tree and run pip-audit

**Files:**
- None modified — verification only

- [ ] **Step 1: Check if langchain-core is still in the dep tree**

Run (for each service):
```bash
cd services/ingestion && pip install -r requirements.txt && pip list | grep -i langchain
cd services/debug && pip install -r requirements.txt && pip list | grep -i langchain
cd services/chat && pip install -r requirements.txt && pip list | grep -i langchain
```

Expected: Only `langchain-text-splitters` appears for ingestion and debug. Nothing langchain-related for chat. If `langchain-core` appears as a transitive dependency of `langchain-text-splitters`, note the version — it needs to be clean of CVE-2026-34070.

- [ ] **Step 2: Run pip-audit with NO ignores**

Run (for each service):
```bash
pip install pip-audit
cd services/ingestion && pip-audit
cd services/debug && pip-audit
cd services/chat && pip-audit
```

Expected: Clean audit — no vulnerabilities found. If any remain, note them for Task 6.

---

### Task 6: Remove `--ignore-vuln` flags from CI

**Files:**
- Modify: `.github/workflows/ci.yml` (lines 487-499)

- [ ] **Step 1: Remove all ignore flags**

Replace the multi-line pip-audit command (lines 487-499):

```yaml
# Before:
      - name: Run pip-audit
        run: >
          pip-audit
          --ignore-vuln CVE-2025-6984
          --ignore-vuln CVE-2025-65106
          --ignore-vuln CVE-2025-68664
          --ignore-vuln CVE-2026-26013
          --ignore-vuln CVE-2025-6985
          --ignore-vuln GHSA-926x-3r5x-gfhw
          --ignore-vuln CVE-2025-71176
          --ignore-vuln GHSA-rr7j-v2q5-chgv
          --ignore-vuln CVE-2026-34070
          --ignore-vuln GHSA-fv5p-p927-qmxr

# After (if all CVEs resolved):
      - name: Run pip-audit
        run: pip-audit
```

If Task 5 found any remaining vulnerabilities, keep those specific `--ignore-vuln` lines with a dated comment explaining the residual risk:

```yaml
      - name: Run pip-audit
        run: >
          pip-audit
          --ignore-vuln <ID>  # 2026-04-16: <reason>, tracked in #<issue>
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "fix(ci): remove pip-audit ignore flags — advisories resolved (#62)"
```

---

### Task 7: Update documentation

**Files:**
- Modify: `services/CLAUDE.md` (line 28)
- Audit: `docs/adr/document-qa/requirements.txt`
- Audit: `docs/adr/document-debugger/requirements.txt`
- Audit: `docs/adr/document-qa/02_pdf_parsing_and_chunking.ipynb`
- Audit: `docs/adr/document-qa/07_wiring_the_endpoints.ipynb`
- Audit: `docs/adr/document-debugger/01_code_aware_chunking.ipynb`

- [ ] **Step 1: Update services/CLAUDE.md**

Replace line 28:
```diff
- - langchain 0.2.x has 5 CVEs that require 0.3.x migration (ignored in pip-audit). Migration tracked as future work.
+ - langchain-community removed (unused). Only langchain-text-splitters is used (ingestion + debug services).
```

- [ ] **Step 2: Update ADR notebook requirements.txt files**

Update `docs/adr/document-qa/requirements.txt` — remove `langchain-community` if present, update `langchain-text-splitters` version. Note: this file also has `pypdf2==3.0.1` (deprecated) — leave that alone, it's out of scope.

Update `docs/adr/document-debugger/requirements.txt` — same: remove `langchain-community`, update `langchain-text-splitters`.

- [ ] **Step 3: Quick-audit ADR notebooks for stale references**

Scan these notebooks for langchain version mentions or stale text:
- `docs/adr/document-qa/02_pdf_parsing_and_chunking.ipynb`
- `docs/adr/document-qa/07_wiring_the_endpoints.ipynb`
- `docs/adr/document-debugger/01_code_aware_chunking.ipynb`

Update any version strings or package name references in markdown cells. Do NOT rewrite code cells unless an import path has actually changed.

- [ ] **Step 4: Commit**

```bash
git add services/CLAUDE.md docs/adr/
git commit -m "docs: update langchain references after dep cleanup"
```

---

### Task 8: Full preflight and final verification

**Files:**
- None modified — verification only

- [ ] **Step 1: Run full preflight**

Run: `make preflight-python && make preflight-security`

Expected: All checks pass. pip-audit runs clean with no (or minimal dated) ignores.

- [ ] **Step 2: Verify acceptance criteria**

Confirm per issue #62: every `--ignore-vuln` in the pip-audit step is either removed or has a dated residual-risk note.

- [ ] **Step 3: Push and watch CI**

Push the branch. Monitor GitHub Actions. All quality + security checks should pass.
