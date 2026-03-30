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
