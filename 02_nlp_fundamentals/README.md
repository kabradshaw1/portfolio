# NLP Fundamentals — Jupyter Notebooks

Core NLP concepts for a developer who's used HuggingFace before but needs a refresher. Each notebook is self-contained — work through it cell-by-cell, retyping code and adding your own explanations.

## Notebooks

| # | File | Topic |
|---|------|-------|
| 0 | `00_tokenization_and_embeddings.ipynb` | How text becomes numbers — tokenizers, subword splitting, sentence embeddings |
| 1 | `01_similarity_and_search.ipynb` | Cosine similarity, semantic vs lexical matching, building a mini search engine |
| 2 | `02_ner_and_classification.ipynb` | Named Entity Recognition (spaCy), sentiment analysis, zero-shot classification |

## Setup

```bash
conda activate gen_ai
pip install -r requirements.txt
python -m spacy download en_core_web_sm
jupyter notebook
```

## Workflow

1. Open the reference notebook
2. Create your own copy — retype code cell-by-cell, run it, add your own comments
3. Commit your version
