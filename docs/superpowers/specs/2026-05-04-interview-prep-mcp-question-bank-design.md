# Interview Prep MCP Question Bank Design

## Purpose

The interview-prep material was split from
`docs/interview-prep/micro1-go-developer` into separate QA and coding-exercise
question banks, but the split is incomplete. Several QA files still contain
implementation exercises, follow-up questions currently import without durable
answer keys, and the MCP services do not expose structured portfolio references
that an agent can cite when coaching answers.

This design makes the question banks operationally useful for MCP-driven
practice:

- QA material contains spoken interview questions, scenario drills, follow-up
  questions, and expected answers.
- Coding-exercise material contains implementation tasks and expected designs.
- The role map remains reference material, not imported QA content.
- The portfolio recall matrix remains imported QA content.
- Imported questions and exercises expose structured repo anchors so agents can
  cite where each concept appears in the portfolio repo.
- Tier 2 follow-ups are contextual follow-ups after strong parent answers, not
  random standalone questions.

## Current State

`go/qa-mcp-service` imports markdown from `docs/interview-prep/qa`. Its parser
loads all direct markdown files except `08-coding-exercises.md`, treats numbered
question headings, scenario headings, exercise headings, and question-like
`###` headings as importable questions, and imports `Follow-ups:` bullets as
tier 2 questions with empty expected answers.

This means embedded `## Coding Exercises` sections in QA files can be imported
as QA prompts. It also means follow-up prompts can be selected without parent
context and without an answer key.

`go/coding-exercises-mcp-service` imports only
`docs/interview-prep/coding-exercises/08-coding-exercises.md`. It already treats
base exercise prompts as `coding_exercise` and exercise follow-ups as `qa`, but
it does not import migrated exercise files beyond that one filename and does not
have structured repo anchors.

`docs/interview-prep/qa/00-role-map.md` is interview strategy and source
context. It is useful reference material, but it is not a QA question bank. In
contrast, `docs/interview-prep/qa/01-portfolio-recall-matrix.md` contains
actual rehearsal questions and should remain imported as category `portfolio`.

## Content Boundaries

QA files under `docs/interview-prep/qa` should contain only:

- high-frequency spoken interview questions,
- scenario drills,
- contextual follow-up questions,
- expected answers,
- structured repo anchors.

Coding-exercise files under `docs/interview-prep/coding-exercises` should
contain:

- implementation prompts,
- expected designs,
- coding-specific follow-up prompts and answers,
- structured repo anchors.

The QA parser must explicitly ignore `00-role-map.md`. The file may remain in
the repository as reference material, but the QA MCP import result must not
include it.

All `## Coding Exercises` sections currently embedded in QA topic files should
be moved into the coding-exercises bank. After the move, QA parser tests should
cover that a `## Coding Exercises` section is skipped if one appears again.
That parser guard prevents future content drift from leaking implementation
tasks into QA.

## Repo Anchors

Markdown should use a small structured convention:

```markdown
Repo anchors:
- `go/payment-service/internal/...` - Applies idempotency and webhook handling.
- `go/pkg/resilience/...` - Shows retry and circuit breaker behavior.
```

Each anchor has:

- `path`: repo-relative file or directory path.
- `note`: a short explanation of why the path applies.

Repo anchors can appear on a base question, scenario, coding exercise, or
follow-up. Follow-ups inherit the parent question's repo anchors unless the
follow-up has its own `Repo anchors:` block. Expected answers and expected
designs should cite these anchors in prose, but the MCP services must also
parse and expose anchors as structured data.

MCP payloads should include `repo_anchors` on:

- `next_question`,
- `next_exercise`,
- submitted QA review payloads when the expected answer is returned,
- submitted coding review payloads when the expected design is returned.

The workflow instructions should tell agents to use `repo_anchors` as the
primary source for portfolio citations. Agents should cite concrete repo paths
when explaining an answer, giving the polished interview answer, or reviewing a
coding exercise implementation.

## Follow-Up Questions

Follow-ups should no longer behave as ambiguous standalone random questions.
They are contextual prompts attached to a parent base question.

Every imported follow-up must have a qualified expected answer. A good follow-up
answer should be shorter than the parent answer but still include:

- the correct concept or tradeoff,
- the portfolio application through explicit or inherited repo anchors,
- interview-ready phrasing,
- any caveat needed to avoid an overbroad answer.

Tier 2 behavior should be:

1. Select a base question from the requested category.
2. Ask the base question first.
3. After the user answers, grade against the parent expected answer.
4. If the parent answer scores well enough, ask a contextual follow-up for that
   same parent.
5. If the parent answer is weak, teach/remediate and move to another base
   question instead of asking a follow-up.

The strong-answer threshold should be score 2 or 3 out of 3. This keeps
follow-ups as depth questions after the user has shown enough understanding for
the follow-up to make sense.

Follow-up selection should be deterministic:

- prefer the first unattempted follow-up for the parent,
- otherwise prefer the weakest previously attempted follow-up for that parent,
- otherwise move to the next base question.

When a follow-up is served, the MCP response should include enough parent
context for the agent to ask it coherently. A follow-up payload should expose at
least `parent_question_id` and `parent_prompt`.

Missing follow-up expected answers should fail content validation or parser
tests. Runtime MCP import should stay usable for local practice, but the repo
should have a test or validation command that prevents committing follow-ups
without answer keys.

Follow-ups should use a parseable block instead of bare bullets:

```markdown
Follow-ups:

#### Follow-up: When is `sync.Map` appropriate?

Fast answer:

> `sync.Map` is useful for read-heavy shared maps with stable keys or disjoint
> key ownership. In this repo, prefer explicit mutexes for normal service state
> because ownership and invariants are easier to test.

Repo anchors:
- `go/...` - Shows the local synchronization choice.
```

Legacy bullet follow-ups may be converted to this form during the content
refresh. New follow-ups should use this form.

## QA MCP Changes

`go/qa-mcp-service` should update its content model, parser, store, and MCP
payloads.

Parser behavior:

- Ignore `00-role-map.md`.
- Skip any `## Coding Exercises` section in QA markdown.
- Parse `Repo anchors:` blocks into structured anchors.
- Attach follow-ups to their parent question with a stable parent link.
- Parse expected answers for follow-ups instead of importing empty answer keys.
- Inherit parent repo anchors for follow-ups without explicit anchors.

Store behavior:

- Persist repo anchors in a child table keyed by question ID.
- Persist follow-up parent linkage with a stable parent source key during import
  and a nullable `parent_question_id` resolved during upsert.
- Preserve existing attempt and feedback history across additive migrations.
- Continue exposing `source_path`, `topic`, `category`, `kind`, `prompt`,
  `expected_answer`, `tier`, and priority.

Selection behavior:

- Tier 1 selects base questions only.
- Tier 2 selects base questions, then returns a parent follow-up after a strong
  parent answer when one is available.
- Follow-ups are not returned as ordinary next questions without parent context.
- Category filters still apply to both the parent and its follow-ups.

Workflow behavior:

- The QA workflow should explicitly tell agents to cite `repo_anchors` in
  explanations and polished interview answers.
- `submit_qa_answer_and_prepare_next` should be able to return a contextual
  follow-up for the just-answered parent when tier 2 is active and the prepared
  score is 2 or 3.
- The returned follow-up should include parent context.
- If the parent answer is weak, the next prompt should be another base question.

## Coding Exercises MCP Changes

`go/coding-exercises-mcp-service` should share the same repo-anchor concept and
support migrated exercise material across multiple markdown files.

Parser behavior:

- Import all direct markdown files under `docs/interview-prep/coding-exercises`
  in lexical order.
- Parse `Repo anchors:` blocks into structured anchors.
- Preserve base exercises as `kind = coding_exercise`.
- Attach coding follow-ups to parent exercises when present.
- Require expected designs for base exercises and expected answers for
  follow-ups.

Store and payload behavior:

- Persist `repo_anchors`.
- Persist parent linkage for follow-ups.
- Include `repo_anchors` in `next_exercise` and review payloads.
- Keep hiding expected designs before review.

Workflow behavior:

- Present base coding exercises as implementation tasks, not prose questions.
- Tell agents to use repo anchors when deciding which files to inspect and when
  explaining how the exercise maps to the portfolio.
- If coding follow-ups are used, ask them only with parent context after the
  implementation review, not as ambiguous standalone prompts.

## Existing Answer Refresh

Existing QA expected answers should be updated so they directly cite where the
concept applies in this portfolio project. The refresh should prioritize:

1. Tier 1 questions.
2. `portfolio` category questions from `01-portfolio-recall-matrix.md`.
3. Follow-up expected answers.
4. Remaining tier 2 and tier 3 material.

The answer refresh should not inflate answers with long code walkthroughs. Each
answer should mention the concept, the repo application, and the most relevant
anchor paths. Deep implementation details belong in the code, not in the answer
bank.

## Testing And Verification

Add or update tests for:

- QA parser ignores `00-role-map.md`.
- QA parser skips embedded `## Coding Exercises` sections.
- QA parser extracts repo anchors.
- QA parser imports follow-ups with expected answers and parent linkage.
- QA selection excludes follow-ups from random standalone selection.
- Tier 2 submit-and-next can return a contextual follow-up after a score of 2 or
  3.
- Tier 2 submit-and-next returns another base question after a weak parent
  answer.
- Coding parser imports migrated exercise material.
- Coding parser extracts repo anchors.
- Store migrations preserve existing data and persist anchors.
- MCP JSON payloads include `repo_anchors` but continue hiding expected answers
  or designs before the user answers or completes a review.
- Workflow resources mention repo-anchor citation behavior and contextual
  follow-up rules.

Before committing implementation changes, run:

```bash
make preflight-go
```

For content-only changes, run the parser and MCP service tests that cover the
question banks. If a local toolchain blocker prevents `make preflight-go`, report
the blocker and leave the remaining verification to CI.

## Acceptance Criteria

- QA MCP does not import coding exercises as QA questions.
- Coding Exercises MCP imports all migrated implementation tasks.
- `00-role-map.md` is not imported as a QA topic.
- `01-portfolio-recall-matrix.md` remains imported as category `portfolio`.
- Imported base questions, follow-ups, scenarios, and coding exercises can
  expose structured `repo_anchors`.
- Existing QA answers cite concrete portfolio repo anchors.
- Imported follow-ups have expected answers.
- Follow-ups have parent context and are not served as random standalone
  questions.
- Tier 2 asks a base question first, then asks a follow-up only after a strong
  answer to that parent.
- MCP workflow instructions require agents to use repo anchors in coaching,
  polished interview answers, and coding review feedback.
- Go parser, store, selection, and MCP payload tests cover the new behavior.
