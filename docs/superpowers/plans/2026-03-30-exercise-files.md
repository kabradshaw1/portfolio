# Exercise Files Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create 6 exercise markdown files — one per Python refresher section — with ipython practice exercises and .py file challenges.

**Architecture:** One markdown file per section in `01_python_refresher/exercises/`. Each contains 5-8 ipython exercises (predict-the-output style, combining 2-3 concepts) and 1 .py challenge (desired output only, no hints).

**Tech Stack:** Markdown files only.

---

## File Structure

```
01_python_refresher/
  exercises/
    exercises_00_environments.md      # ipython exercises + env_check.py challenge
    exercises_01_data_structures.md   # ipython exercises + data_structures.py challenge
    exercises_02_oop_patterns.md      # ipython exercises + oop_patterns.py challenge
    exercises_03_async_basics.md      # ipython exercises + async_basics.py challenge
    exercises_04_type_hints.md        # ipython exercises + type_hints.py challenge
    exercises_05_data_processing.md   # ipython exercises + data_processing.py challenge
```

---

### Task 1: Create exercises directory and exercises_00_environments.md

**Files:**
- Create: `01_python_refresher/exercises/exercises_00_environments.md`

- [ ] **Step 1: Create the directory**

```bash
mkdir -p /Users/kylebradshaw/repos/gen_ai_engineer/01_python_refresher/exercises
```

- [ ] **Step 2: Write the exercise file**

Write `01_python_refresher/exercises/exercises_00_environments.md` with this exact content:

```markdown
# Exercises: Environments

After completing the reference notebook, test yourself with these.

## ipython Exercises

Type each into ipython. **Predict the output BEFORE you hit enter.**

### 1. Module search path

```python
import sys
print(len(sys.path) > 0)
print(sys.path[0] == '')
```

### 2. Type of a module

```python
import os
print(type(os))
print(type(os.path))
```

### 3. Reloading a module

```python
import importlib
import math
math.pi = 3
print(math.pi)
importlib.reload(math)
print(math.pi)
```

### 4. The __name__ trick

```python
def main():
    return "running"

print(__name__)
print(main())
```

### 5. sys.modules cache

```python
import sys
import json
print('json' in sys.modules)
del sys.modules['json']
print('json' in sys.modules)
import json
print('json' in sys.modules)
```

### 6. Chained magic commands

```python
%timeit -n 1 -r 1 sum(range(100))
%timeit -n 1 -r 1 sum(list(range(100)))
```

## .py Challenge

Create `env_check.py` that produces this exact output:

```
=== Environment Report ===
Python: 3.11
Platform: [your platform, e.g. macOS-15.3.2-arm64-arm-64bit]
Executable: [your python path]

Installed packages:
  numpy ✓
  pandas ✓
  jupyter ✓

sys.path entries: [number]
```

Notes: The bracketed values will vary by machine — match the format, not the exact values. Use only the standard library plus the packages listed.
```

- [ ] **Step 3: Commit**

```bash
git add 01_python_refresher/exercises/
git commit -m "exercises: add exercises_00_environments"
```

---

### Task 2: Create exercises_01_data_structures.md

**Files:**
- Create: `01_python_refresher/exercises/exercises_01_data_structures.md`

- [ ] **Step 1: Write the exercise file**

Write `01_python_refresher/exercises/exercises_01_data_structures.md` with this exact content:

```markdown
# Exercises: Data Structures

After completing the reference notebook, test yourself with these.

## ipython Exercises

Type each into ipython. **Predict the output BEFORE you hit enter.**

### 1. Aliasing through a function

```python
def append_to(item, target=[]):
    target.append(item)
    return target

print(append_to(1))
print(append_to(2))
print(append_to(3))
```

### 2. Comprehension with condition and transform

```python
words = ["hello", "WORLD", "Python", "GO", "ts"]
result = {w.lower(): len(w) for w in words if len(w) > 2}
print(result)
```

### 3. Nested unpacking

```python
data = [(1, (2, 3)), (4, (5, 6))]
result = [a + b + c for a, (b, c) in data]
print(result)
```

### 4. Set operations chain

```python
a = {1, 2, 3, 4, 5}
b = {4, 5, 6, 7}
c = {5, 6, 8, 9}
print(a & b | c)
print((a & b) | c)
print(a & (b | c))
```

### 5. Generator exhaustion

```python
gen = (x for x in range(3))
print(list(gen))
print(list(gen))
print(sum(x for x in range(3)))
```

### 6. Dict merge operators

```python
defaults = {"color": "red", "size": 10, "visible": True}
overrides = {"size": 20, "name": "box"}
merged = defaults | overrides
print(merged)
print(len(merged))
```

### 7. Slice assignment with different length

```python
a = [0, 1, 2, 3, 4]
a[1:4] = [10, 20, 30, 40, 50]
print(a)
print(len(a))
```

### 8. Tuple as dict key with computation

```python
grid = {}
for x in range(3):
    for y in range(3):
        grid[(x, y)] = x * 3 + y

diag = [grid[(i, i)] for i in range(3)]
print(diag)
```

## .py Challenge

Create `data_cruncher.py` that produces this exact output:

```
Original: [3, 1, 4, 1, 5, 9, 2, 6, 5, 3, 5]
Unique sorted: [1, 2, 3, 4, 5, 6, 9]
Frequency: {1: 2, 2: 1, 3: 2, 4: 1, 5: 3, 6: 1, 9: 1}
Most common: 5 (3 times)
Pairs that sum to 10: [(1, 9), (4, 6), (5, 5)]
Squared evens: [4, 36]
Running total: [3, 4, 8, 9, 14, 23, 25, 31, 36, 39, 44]
```

No hints. No function signatures. Figure it out from the output.
```

- [ ] **Step 2: Commit**

```bash
git add 01_python_refresher/exercises/exercises_01_data_structures.md
git commit -m "exercises: add exercises_01_data_structures"
```

---

### Task 3: Create exercises_02_oop_patterns.md

**Files:**
- Create: `01_python_refresher/exercises/exercises_02_oop_patterns.md`

- [ ] **Step 1: Write the exercise file**

Write `01_python_refresher/exercises/exercises_02_oop_patterns.md` with this exact content:

```markdown
# Exercises: OOP Patterns

After completing the reference notebook, test yourself with these.

## ipython Exercises

Type each into ipython. **Predict the output BEFORE you hit enter.**

### 1. Method resolution order

```python
class A:
    def greet(self):
        return "A"

class B(A):
    def greet(self):
        return "B"

class C(A):
    def greet(self):
        return "C"

class D(B, C):
    pass

print(D().greet())
print([cls.__name__ for cls in D.__mro__])
```

### 2. Property with inheritance

```python
class Base:
    def __init__(self):
        self._value = 10

    @property
    def value(self):
        return self._value

class Child(Base):
    @Base.value.setter
    def value(self, v):
        self._value = v * 2

c = Child()
print(c.value)
c.value = 5
print(c.value)
```

### 3. Dunder len and bool

```python
class Bag:
    def __init__(self, items):
        self.items = items
    def __len__(self):
        return len(self.items)

empty = Bag([])
full = Bag([1, 2, 3])
print(bool(empty))
print(bool(full))
print(len(empty))
```

### 4. __eq__ without __hash__

```python
class Point:
    def __init__(self, x, y):
        self.x = x
        self.y = y
    def __eq__(self, other):
        return self.x == other.x and self.y == other.y

p1 = Point(1, 2)
p2 = Point(1, 2)
print(p1 == p2)
try:
    s = {p1, p2}
    print(len(s))
except TypeError as e:
    print(f"Error: {e}")
```

### 5. Class vs instance attributes

```python
class Counter:
    count = 0
    def __init__(self):
        Counter.count += 1
        self.id = Counter.count

a = Counter()
b = Counter()
c = Counter()
print(f"a.id={a.id}, b.id={b.id}, c.id={c.id}")
print(f"Counter.count={Counter.count}")
print(f"a.count={a.count}")
```

### 6. ABC enforcement timing

```python
from abc import ABC, abstractmethod

class Shape(ABC):
    @abstractmethod
    def area(self):
        pass

class Circle(Shape):
    pass

print(type(Circle))
try:
    c = Circle()
except TypeError as e:
    print(f"Error: {e}")
```

## .py Challenge

Create `inventory.py` that produces this exact output:

```
=== Inventory System ===

Added: Laptop ($999.99, qty: 5)
Added: Mouse ($29.99, qty: 50)
Added: Keyboard ($79.99, qty: 30)
Added: Monitor ($349.99, qty: 10)

Total items: 4
Total value: $12,149.55

Most expensive: Laptop ($999.99)
Cheapest: Mouse ($29.99)

Items over $100:
  - Laptop: $999.99 x 5 = $4,999.95
  - Monitor: $349.99 x 10 = $3,499.90

Inventory sorted by total value (desc):
  1. Laptop — $4,999.95
  2. Monitor — $3,499.90
  3. Keyboard — $2,399.70
  4. Mouse — $1,499.50
```

No hints. No function signatures. Figure it out from the output.
```

- [ ] **Step 2: Commit**

```bash
git add 01_python_refresher/exercises/exercises_02_oop_patterns.md
git commit -m "exercises: add exercises_02_oop_patterns"
```

---

### Task 4: Create exercises_03_async_basics.md

**Files:**
- Create: `01_python_refresher/exercises/exercises_03_async_basics.md`

- [ ] **Step 1: Write the exercise file**

Write `01_python_refresher/exercises/exercises_03_async_basics.md` with this exact content:

```markdown
# Exercises: Async Basics

After completing the reference notebook, test yourself with these.

## ipython Exercises

Type each into ipython. **Predict the output BEFORE you hit enter.**

### 1. Coroutine without await

```python
import asyncio

async def double(x):
    return x * 2

coro = double(5)
print(type(coro))
result = await coro
print(result)
```

### 2. Gather ordering

```python
import asyncio

async def delayed_value(val, delay):
    await asyncio.sleep(delay)
    return val

results = await asyncio.gather(
    delayed_value("slow", 0.3),
    delayed_value("fast", 0.1),
    delayed_value("medium", 0.2),
)
print(results)
```

### 3. Task cancellation

```python
import asyncio

async def long_task():
    try:
        await asyncio.sleep(10)
        return "done"
    except asyncio.CancelledError:
        return "cancelled"

task = asyncio.create_task(long_task())
await asyncio.sleep(0.1)
task.cancel()
try:
    result = await task
except asyncio.CancelledError:
    result = "caught outside"
print(result)
```

### 4. Semaphore fairness

```python
import asyncio

sem = asyncio.Semaphore(1)
order = []

async def worker(name, delay):
    async with sem:
        order.append(f"{name}-start")
        await asyncio.sleep(delay)
        order.append(f"{name}-end")

await asyncio.gather(
    worker("A", 0.2),
    worker("B", 0.1),
    worker("C", 0.1),
)
print(order)
```

### 5. Return exceptions vs raise

```python
import asyncio

async def fail():
    raise ValueError("boom")

async def succeed():
    return "ok"

results = await asyncio.gather(
    succeed(),
    fail(),
    succeed(),
    return_exceptions=True,
)
print([type(r).__name__ for r in results])
print([str(r) for r in results])
```

### 6. Async generator collection

```python
import asyncio

async def trickle(n):
    for i in range(n):
        await asyncio.sleep(0.05)
        yield i * i

squares = [x async for x in trickle(5)]
print(squares)
```

## .py Challenge

Create `async_pipeline.py` that produces this exact output when run with `python async_pipeline.py`:

```
Starting pipeline...

[fetch] Fetching user_001... done (0.3s)
[fetch] Fetching user_002... done (0.1s)
[fetch] Fetching user_003... done (0.2s)
All fetches complete in 0.3s

[process] Processing user_001... done
[process] Processing user_002... done
[process] Processing user_003... done

Pipeline results:
  user_001: score=82 (grade: B)
  user_002: score=95 (grade: A)
  user_003: score=67 (grade: D)

Pipeline finished in ~0.3s (concurrent fetch + sequential process)
```

Notes: Timing values are approximate. The fetches must run concurrently (total fetch time ≈ max delay, not sum). Processing is sequential. Grading: A=90+, B=80+, C=70+, D=60+, F=below 60.

No hints. No function signatures. Figure it out from the output.
```

- [ ] **Step 2: Commit**

```bash
git add 01_python_refresher/exercises/exercises_03_async_basics.md
git commit -m "exercises: add exercises_03_async_basics"
```

---

### Task 5: Create exercises_04_type_hints.md

**Files:**
- Create: `01_python_refresher/exercises/exercises_04_type_hints.md`

- [ ] **Step 1: Write the exercise file**

Write `01_python_refresher/exercises/exercises_04_type_hints.md` with this exact content:

```markdown
# Exercises: Type Hints

After completing the reference notebook, test yourself with these.

## ipython Exercises

Type each into ipython. **Predict the output BEFORE you hit enter.**

### 1. Runtime doesn't care

```python
def strict_add(a: int, b: int) -> int:
    return a + b

print(strict_add([1, 2], [3, 4]))
print(type(strict_add([1, 2], [3, 4])))
```

### 2. isinstance vs type hints

```python
from typing import Protocol

class Printable(Protocol):
    def __str__(self) -> str: ...

x = 42
print(isinstance(x, Printable))
```

### 3. TypedDict at runtime

```python
from typing import TypedDict

class Config(TypedDict):
    host: str
    port: int

c: Config = {"host": "localhost", "port": 8080}
print(type(c))
print(c.__class__.__name__)
c["extra"] = True
print(len(c))
```

### 4. TypeVar identity

```python
from typing import TypeVar

T = TypeVar('T')

def identity(x: T) -> T:
    return x

result = identity("hello")
print(result)
print(type(result).__name__)
result2 = identity(42)
print(result2)
```

### 5. get_type_hints introspection

```python
from typing import get_type_hints, Optional

def process(name: str, age: int, email: Optional[str] = None) -> bool:
    return True

hints = get_type_hints(process)
print(list(hints.keys()))
print(hints['return'].__name__)
```

### 6. Annotations are just data

```python
x: int = "not an int"
y: str = 42

print(x, y)
print(__annotations__)
print(type(__annotations__['x']).__name__)
```

## .py Challenge

Create `type_safe.py` that produces this exact output:

```
=== Type Registry ===

Registered: IntValidator (validates: int)
Registered: StringValidator (validates: str)
Registered: ListValidator (validates: list)

Validating 42 with IntValidator: ✓ valid
Validating "hello" with IntValidator: ✗ invalid (expected int, got str)
Validating "hello" with StringValidator: ✓ valid
Validating [1, 2] with ListValidator: ✓ valid
Validating (1, 2) with ListValidator: ✗ invalid (expected list, got tuple)

Registry contents:
  int -> IntValidator
  str -> StringValidator
  list -> ListValidator

Lookup validator for int: IntValidator
Lookup validator for float: None (not registered)
```

Notes: Must pass `mypy --strict` with no errors. Use Protocol, TypeVar, or generics — not just isinstance checks with string formatting.

No hints. No function signatures. Figure it out from the output.
```

- [ ] **Step 2: Commit**

```bash
git add 01_python_refresher/exercises/exercises_04_type_hints.md
git commit -m "exercises: add exercises_04_type_hints"
```

---

### Task 6: Create exercises_05_data_processing.md

**Files:**
- Create: `01_python_refresher/exercises/exercises_05_data_processing.md`

- [ ] **Step 1: Write the exercise file**

Write `01_python_refresher/exercises/exercises_05_data_processing.md` with this exact content:

```markdown
# Exercises: Data Processing

After completing the reference notebook, test yourself with these.

## ipython Exercises

Type each into ipython. **Predict the output BEFORE you hit enter.**

### 1. NumPy view mutation

```python
import numpy as np
a = np.arange(10)
b = a[3:7]
b[0] = 99
print(a)
```

### 2. Boolean mask chaining

```python
import numpy as np
a = np.array([15, 22, 8, 31, 12, 45, 3, 28])
mask = (a > 10) & (a < 30) & (a % 2 == 0)
print(a[mask])
print(mask.sum())
```

### 3. Broadcasting

```python
import numpy as np
a = np.array([[1], [2], [3]])
b = np.array([10, 20, 30])
print(a + b)
print((a + b).shape)
```

### 4. DataFrame boolean indexing chain

```python
import pandas as pd
df = pd.DataFrame({
    "name": ["A", "B", "C", "D", "E"],
    "score": [85, 92, 78, 95, 88],
    "group": ["x", "y", "x", "y", "x"]
})
result = df[df["group"] == "x"]["score"].mean()
print(result)
```

### 5. GroupBy transform vs aggregate

```python
import pandas as pd
df = pd.DataFrame({
    "team": ["a", "a", "b", "b"],
    "pts": [10, 20, 30, 40]
})
print(df.groupby("team")["pts"].transform("mean").tolist())
print(df.groupby("team")["pts"].agg("mean").tolist())
```

### 6. NumPy where

```python
import numpy as np
a = np.array([1, -2, 3, -4, 5])
result = np.where(a > 0, a * 10, 0)
print(result)
```

### 7. Pandas apply with axis

```python
import pandas as pd
df = pd.DataFrame({
    "a": [1, 2, 3],
    "b": [4, 5, 6]
})
row_sums = df.apply(sum, axis=1).tolist()
col_sums = df.apply(sum, axis=0).tolist()
print(row_sums)
print(col_sums)
```

## .py Challenge

Create `sales_report.py` that produces this exact output:

```
=== Q1 Sales Report ===

Raw data: 15 transactions loaded

By region:
  North: $12,450.00 (5 transactions, avg $2,490.00)
  South: $8,320.50 (4 transactions, avg $2,080.12)
  East: $15,890.75 (3 transactions, avg $5,296.92)
  West: $6,275.00 (3 transactions, avg $2,091.67)

Top 3 transactions:
  1. East — $7,500.00
  2. East — $5,200.00
  3. North — $4,100.00

Monthly trend:
  Jan: $14,250.00
  Feb: $12,890.25
  Mar: $15,796.00

Growth: Jan→Feb -9.5%, Feb→Mar +22.5%

Regions above average ($10,734.06):
  - North ($12,450.00, +16.0%)
  - East ($15,890.75, +48.1%)
```

Notes: You must construct the data yourself (hardcode a list of dicts or build a DataFrame directly). Use pandas for all aggregation. Dollar amounts are formatted with commas and 2 decimal places. Percentages are rounded to 1 decimal place.

No hints. No function signatures. Figure it out from the output.
```

- [ ] **Step 2: Commit**

```bash
git add 01_python_refresher/exercises/exercises_05_data_processing.md
git commit -m "exercises: add exercises_05_data_processing"
```

---

## Summary

| Task | File | Commit |
|------|------|--------|
| 1 | `exercises_00_environments.md` | `exercises: add exercises_00_environments` |
| 2 | `exercises_01_data_structures.md` | `exercises: add exercises_01_data_structures` |
| 3 | `exercises_02_oop_patterns.md` | `exercises: add exercises_02_oop_patterns` |
| 4 | `exercises_03_async_basics.md` | `exercises: add exercises_03_async_basics` |
| 5 | `exercises_04_type_hints.md` | `exercises: add exercises_04_type_hints` |
| 6 | `exercises_05_data_processing.md` | `exercises: add exercises_05_data_processing` |
