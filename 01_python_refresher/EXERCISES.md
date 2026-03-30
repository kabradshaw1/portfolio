# Python Refresher — Exercise Guide

Work through these exercises in order. Each one builds a standalone script. Run your code frequently — don't write more than 10-15 lines without testing.

---

## Exercise 1: `data_structures.py`

**Goal:** Demonstrate fluency with lists, dicts, sets, comprehensions, and generators.

### Tasks

1. **Lists** — Create a list of numbers. Practice:
   - Slicing: reverse it, grab every other element, get the last 3
   - `sorted()` vs `.sort()` — what's the difference?
   - A list comprehension that filters (e.g., only evens)
   - A nested comprehension that flattens a list of lists

2. **Dicts** — Create a dict mapping names to scores. Practice:
   - A dict comprehension that filters entries (e.g., scores above 80)
   - Merging two dicts with `{**a, **b}`
   - Grouping items using `.setdefault()` — e.g., group students by subject
   - Sorting a dict by value using `sorted()` with a `key` lambda

3. **Sets** — Create two sets (e.g., python_devs, ml_devs). Practice:
   - Intersection (`&`), union (`|`), difference (`-`), symmetric difference (`^`)

4. **Generators** — Practice:
   - A generator expression (like a list comprehension but with `()`)
   - A generator function using `yield` — try Fibonacci
   - Chain two generators together (one feeds into another)

### What to look for
- Do you know when to use a list vs a set vs a dict?
- Can you write a comprehension without looking up the syntax?
- Do you understand that generators are lazy (only compute values when asked)?

---

## Exercise 2: `oop_patterns.py`

**Goal:** Practice classes, inheritance, ABCs, and dataclasses.

### Tasks

1. **Abstract Base Class** — Create an ABC called `DocumentProcessor` with:
   - An abstract method `process(text: str) -> str`
   - An abstract method `name() -> str`
   - Write 2 concrete subclasses (e.g., a Summarizer that truncates text, a KeywordExtractor that finds frequent words)

2. **Dataclass** — Create a `@dataclass` for a pipeline config that holds:
   - A name (str)
   - A list of processors
   - A `run()` method that passes text through each processor and collects results

3. **Inheritance with `super()`** — Subclass one of your processors and override `process()`. Call `super().process()` as a fallback.

### What to look for
- Can you define an ABC and get a `TypeError` when you forget to implement an abstract method? Try it.
- Do you understand what `@dataclass` gives you for free (`__init__`, `__repr__`, `__eq__`)?
- Do you know when to use `super()` and what it resolves to?

---

## Exercise 3: `async_basics.py`

**Goal:** Understand async/await and why it matters for FastAPI.

### Tasks

1. **Basic coroutine** — Write an async function that simulates a slow API call using `await asyncio.sleep(delay)` and returns a result dict.

2. **Sequential vs concurrent** — Call your function 3 times:
   - First sequentially (await one, then the next, then the next)
   - Then concurrently with `asyncio.gather()`
   - Use `time.perf_counter()` to measure both. Print the speedup.

3. **Timeout handling** — Use `asyncio.wait_for()` to set a timeout on a slow call. Catch `asyncio.TimeoutError`.

4. **Producer-consumer** — Use `asyncio.Queue`:
   - A producer coroutine that puts items on the queue
   - A consumer coroutine that gets items off the queue
   - Run them concurrently with `asyncio.create_task()`

5. **Entry point** — Use `asyncio.run(main())` to run everything.

### What to look for
- `async def` creates a coroutine, `await` suspends it. No threads involved.
- `asyncio.gather()` runs coroutines concurrently on a single thread — it switches between them at `await` points.
- FastAPI uses this model: while one request awaits a DB call, another request can be handled.

---

## Exercise 4: `type_hints.py`

**Goal:** Practice type annotations, generics, and Protocol.

### Tasks

1. **Annotate a function** — Write a function with full type hints on args and return. E.g., `def process_scores(scores: dict[str, float], threshold: float = 0.5) -> list[str]`

2. **TypeVar** — Import `TypeVar` from `typing`. Create a generic function like:
   - `first_or_default(items: list[T], default: T) -> T`
   - `chunk_list(items: list[T], size: int) -> list[list[T]]`

3. **Protocol** — Define a `Protocol` class (e.g., `Embeddable`) with a method signature. Then write 2 unrelated classes that both happen to have that method. Write a function that accepts `Embeddable` — both classes should work without inheriting from it.

4. **Generic class** — Create a `@dataclass` that's `Generic[T]`, e.g., `Result(Generic[T])` with a `value: T` field. Instantiate it with different types.

### What to look for
- Protocol is *structural* subtyping ("duck typing with teeth") — unlike ABC, classes don't need to explicitly inherit from it.
- `TypeVar` lets you write functions that preserve input types in their return types.
- Modern Python (3.10+) lets you write `list[str]` directly instead of `List[str]` from typing.

---

## Exercise 5: `data_processing.py`

**Goal:** Demonstrate pandas/numpy for data manipulation.

### Tasks

1. **NumPy basics** — Create arrays, practice:
   - Vectorized operations (add, multiply, compare arrays without loops)
   - Reshaping, slicing, boolean indexing
   - `np.mean()`, `np.std()`, `np.dot()`

2. **Create a DataFrame** — Build a pandas DataFrame from a dict (e.g., employee data with name, department, salary columns). Practice:
   - Filtering rows: `df[df["salary"] > 70000]`
   - Groupby + aggregation: `df.groupby("department")["salary"].mean()`
   - Adding a computed column
   - Sorting by a column

3. **Data cleaning** — Create a messy DataFrame (some `None` values, duplicate rows). Practice:
   - `df.dropna()`, `df.fillna()`
   - `df.drop_duplicates()`
   - `df.dtypes` and type casting with `.astype()`

### What to look for
- NumPy is about avoiding Python loops — vectorized ops are 10-100x faster.
- Pandas builds on NumPy for tabular data. Groupby is the most important operation to master.
- Can you read a pandas chain like `df.groupby("x")["y"].mean().sort_values()` and know what each step does?

---

## General Tips

- **Run early, run often.** Print intermediate results.
- **Break things on purpose.** Forget an abstract method, pass the wrong type, skip an `await`. See what the error looks like so you recognize it later.
- **Keep each script under 100 lines.** These are exercises, not production code.
- **Add a `if __name__ == "__main__":` block** that calls a `demo()` or `main()` function so each script runs standalone.

When you're done with all 5, let me know and we'll move on to the NLP section.
