# NLP Fundamentals Section

**Date:** 2026-03-30
**Status:** Draft

## Problem

Kyle needs to refresh NLP fundamentals for the Gen AI Engineer portfolio. He has some prior exposure to HuggingFace and possibly spaCy/sentence-transformers but it's rusty. The job posting lists NLP as "good to have."

## Solution

Three reference Jupyter notebooks in `02_nlp_fundamentals/`, following the same format as the Python refresher. Kyle retypes them cell-by-cell. Each notebook combines related topics to keep the count manageable and reduce overhead.

## Structure

```
02_nlp_fundamentals/
  README.md
  requirements.txt
  00_tokenization_and_embeddings.ipynb
  01_similarity_and_search.ipynb
  02_ner_and_classification.ipynb
```

## Dependencies

**requirements.txt:**
```
jupyter
transformers
sentence-transformers
torch
spacy
scikit-learn
```

**Post-install:** `python -m spacy download en_core_web_sm`

## Notebook Format

Same pattern as Python refresher:
1. Markdown cell — Go/TS comparison framing
2. Code cell — complete, runnable example
3. Code cell — experiment prompt
4. Markdown cell — "in your own words" prompt

Top cell: title, goal, prereqs. Bottom cell: recap checklist.

## Notebook Content

### 00_tokenization_and_embeddings.ipynb

**Theme:** How text becomes numbers that machines can work with.

**Go/TS framing:** "In Go, you'd work with `[]byte` or `[]string` — text is just data you parse. In NLP, text needs to be converted into numeric representations before any model can touch it."

**Sections:**
1. Word tokenization vs subword (BPE) — why splitting on spaces doesn't cut it
2. HuggingFace tokenizers — `AutoTokenizer`, `.tokenize()`, `.encode()`, `.decode()`
3. Comparing tokenizers — BERT vs GPT-2, different vocabularies
4. Special tokens — `[CLS]`, `[SEP]`, `[PAD]` and why they exist
5. Sentence embeddings — `SentenceTransformer`, `.encode()`
6. Embedding properties — fixed-size output, deterministic, captures meaning not just words
7. Comparing models — `all-MiniLM-L6-v2` vs `all-mpnet-base-v2`, different dimensions

### 01_similarity_and_search.ipynb

**Theme:** How to compare and find similar text — the foundation of RAG.

**Go/TS framing:** "In your Go RAG project, you sent text to an LLM and got answers back. But how did the system know which documents were relevant? This is the retrieval part — cosine similarity over embeddings."

**Sections:**
1. Cosine similarity from scratch — dot product, magnitudes, the formula
2. Verify with sklearn — `cosine_similarity` should match your implementation
3. Semantic vs lexical similarity — sentences that share words but differ in meaning, and vice versa
4. Building a mini search engine — embed documents, embed a query, rank by similarity
5. Similarity matrix — compare all documents against each other, see clusters form

### 02_ner_and_classification.ipynb

**Theme:** Extracting structure from unstructured text.

**Go/TS framing:** "In a web service, you parse JSON into structs. NER does something similar for natural language — it finds the structured entities hiding in unstructured text. Classification assigns labels to whole documents — like routing requests to handlers."

**Sections:**
1. spaCy NER — load model, process text, iterate `.ents`
2. Entity types — PERSON, ORG, GPE, DATE, MONEY — what they mean
3. NER on different texts — news, technical, personal — where it fails
4. Sentiment analysis — HuggingFace `pipeline("sentiment-analysis")`
5. Zero-shot classification — `pipeline("zero-shot-classification")` with custom labels
6. Confidence and thresholds — when to trust model predictions

## Repo Updates

- **CLAUDE.md:** Add `02_nlp_fundamentals/` to project structure section. Same authorship rules — Claude generates reference notebooks, Kyle retypes.
- **README.md:** Change NLP row from "Planned" to "In progress."

## Authorship Rules

Same as Python refresher: Claude generates reference notebooks as learning materials. Kyle's deliverable is his own retyped copy with his own comments.
