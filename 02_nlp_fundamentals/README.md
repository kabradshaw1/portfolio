# NLP Fundamentals

Demonstrations of core NLP concepts using industry-standard libraries. Each script focuses on one concept; the unified notebook ties them all together.

## Scripts

| File | Concept | Library |
|------|---------|---------|
| `tokenization.py` | Word/subword tokenization | HuggingFace tokenizers |
| `embeddings.py` | Text embeddings | sentence-transformers |
| `cosine_similarity.py` | Semantic similarity | sentence-transformers, sklearn |
| `ner.py` | Named Entity Recognition | spaCy |
| `text_classification.py` | Text classification | HuggingFace transformers |
| `nlp_unified_notebook.ipynb` | All concepts combined | All of the above |

## Setup

```bash
pip install -r requirements.txt
python -m spacy download en_core_web_sm
python tokenization.py
```
