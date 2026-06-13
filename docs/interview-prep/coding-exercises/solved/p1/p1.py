from collections import Counter

items = [1, 2, 2, 3, 3, 3, 4]
counts = Counter(items)

counts = {}

for x in items:
    counts[x] = counts.get(x, 0) + 1

def count_duplicated_values(items):
    counts = Counter(items)
    return sum(1 for c in counts.values() if c > 1)

def count_extra_occurrences(items):
    counts = Counter(items)
    return sum(c - 1 for c in counts.values() if c > 1)