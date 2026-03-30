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
