# Exercise Markdown Files for Python Refresher

**Date:** 2026-03-30
**Status:** Draft

## Problem

The reference notebooks teach concepts, but there's no way to test understanding. Kyle needs practice exercises that go beyond the notebooks — combining concepts in new ways — plus a .py file challenge per section to practice writing standalone scripts.

## Solution

One exercise markdown file per notebook section, containing:
1. **5-8 ipython exercises** — type into REPL, predict output before running, each combines 2-3 concepts
2. **1 .py file challenge** — given only the desired terminal output, reverse-engineer the script

## File Structure

```
01_python_refresher/
  exercises/
    exercises_00_environments.md
    exercises_01_data_structures.md
    exercises_02_oop_patterns.md
    exercises_03_async_basics.md
    exercises_04_type_hints.md
    exercises_05_data_processing.md
```

## Exercise File Format

```markdown
# Exercises: [Topic Name]

After completing the reference notebook, test yourself with these.

## ipython Exercises

Type each into ipython. **Predict the output BEFORE you hit enter.**

### 1. [Short title]

\```python
[code to type]
\```

### 2. [Short title]
...

## .py Challenge

Create `[filename].py` that produces this exact output:

\```
[exact terminal output]
\```

No hints. No function signatures. Figure it out from the output.
```

## Content Guidelines

- **Difficulty:** One step beyond the notebook — exercises combine 2-3 concepts in ways the notebook didn't show directly
- **Independence:** Each exercise stands alone, no dependencies between them
- **ipython exercises:** Ask to predict output before running — forces engagement over passive typing
- **`.py` challenge:** Output-only — no descriptions, no function signatures, pure reverse-engineering
- **No answers:** Exercise files contain no solutions

## Per-Section Content

### exercises_00_environments.md
- ipython: `sys.path` exploration, magic command combos, kernel state gotchas, import mechanics
- .py challenge: Script that prints formatted environment diagnostic info

### exercises_01_data_structures.md
- ipython: Nested comprehensions, dict/set combos, generator chaining, aliasing traps, slice edge cases
- .py challenge: Script that processes data using multiple data structures together

### exercises_02_oop_patterns.md
- ipython: Dunder method interactions, ABC + property combos, inheritance edge cases, class method resolution
- .py challenge: Script with classes that produce specific formatted output

### exercises_03_async_basics.md
- ipython: Task timing puzzles, error propagation in gather, semaphore ordering, async generator combos
- .py challenge: Async script with specific concurrent output pattern

### exercises_04_type_hints.md
- ipython: Protocol satisfaction puzzles, generic function edge cases, TypedDict nesting, runtime type behavior
- .py challenge: Script with typed functions that mypy must pass cleanly

### exercises_05_data_processing.md
- ipython: Boolean mask chains, groupby + apply combos, NumPy view mutation puzzles, DataFrame merge/join
- .py challenge: Script that processes data and prints a formatted summary table
