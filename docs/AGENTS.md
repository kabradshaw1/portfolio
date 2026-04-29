# Documentation

Docs are pull-based reference material. Do not read or summarize `docs/`
broadly during routine project discovery.

## Loading Policy

- Load ADRs only for architecture decisions or when explicitly referenced.
- Load runbooks only for matching operational tasks.
- `docs/product-catalog/` contains RAG/test fixture content, not engineering
  onboarding context.
- `docs/superpowers/` contains generated specs and plans. Keep it ignored
  except when the user or a skill explicitly asks for a spec or plan.
- Prefer targeted `rg` searches that respect `.ignore` over `find` or broad
  directory scans.

## Adding Documentation

Use `docs/adr/template-adr.md` for new ADRs. Add deep decisions and references
to `docs/`, repeated procedures to a skill, and directory-scoped rules to the
nearest `AGENTS.md`.

## Handoffs, Specs, And Plans

For handoffs, specs, and plans, read the TL;DR, status, or current-task
sections first, then jump only to referenced sections needed for the immediate
next step. Do not stream thousands of lines of a plan, spec, or handoff unless
Kyle explicitly asks for a full review.
