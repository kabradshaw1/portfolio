# NLP Fundamentals — Exercise Guide

These exercises build on each other conceptually. Do them in order — each one introduces ideas the next one uses.

---

## Exercise 1: `tokenization.py`

**Goal:** Understand how text gets broken into pieces before any model can use it.

### Background
Models don't see words — they see token IDs (integers). How text gets split into tokens affects everything downstream. There are three main strategies:

- **Word-level:** split on spaces/punctuation. Simple but can't handle unknown words.
- **Character-level:** every character is a token. Handles anything but sequences get very long.
- **Subword (BPE/WordPiece):** the sweet spot. Splits common words whole, breaks rare words into pieces. This is what modern LLMs use.

### Tasks

1. **Manual word tokenization** — Split a sentence on whitespace. Then try handling punctuation (e.g., "don't" → "do", "n't"). See why this is hard to get right.

2. **HuggingFace tokenizer** — Install `transformers` and load a pretrained tokenizer:
   ```python
   from transformers import AutoTokenizer
   tokenizer = AutoTokenizer.from_pretrained("bert-base-uncased")
   ```
   - Tokenize a sentence with `tokenizer.tokenize()` — look at how it splits words
   - Get token IDs with `tokenizer.encode()` — these are what the model actually sees
   - Decode IDs back to text with `tokenizer.decode()` — verify round-trip
   - Try a made-up word like "unfrigginbelievable" and see how subword tokenization handles it

3. **Compare tokenizers** — Load a second tokenizer (e.g., `gpt2`) and tokenize the same sentences. Notice how different models split text differently.

4. **Special tokens** — Print `tokenizer.special_tokens_map`. What are `[CLS]`, `[SEP]`, `[PAD]`? Why do they exist?

### What to look for
- Subword tokenizers break unknown words into known pieces (e.g., "unfriggin" → "un", "##fri", "##gg", "##in")
- Token count ≠ word count. This matters for LLM context windows.
- Different models use different vocabularies and tokenization strategies.

---

## Exercise 2: `embeddings.py`

**Goal:** Understand how text becomes a numeric vector that captures meaning.

### Background
An embedding is a dense vector (list of floats) that represents text in a high-dimensional space. Texts with similar meanings end up close together in that space. This is the foundation of semantic search, RAG, and most modern NLP.

### Tasks

1. **Generate embeddings** — Use `sentence-transformers`:
   ```python
   from sentence_transformers import SentenceTransformer
   model = SentenceTransformer("all-MiniLM-L6-v2")
   ```
   - Embed a single sentence with `model.encode("your text")`
   - Print the shape and first 10 values. Notice it's a fixed-size float array regardless of input length.

2. **Embed multiple sentences** — Pass a list of sentences to `model.encode()`. Compare:
   - Two sentences that mean the same thing differently ("The cat sat on the mat" / "A feline rested on the rug")
   - Two sentences that share words but mean different things ("bank of the river" / "bank account balance")

3. **Visualize distance** — Print the vectors side-by-side (first 10 dimensions is fine). Eyeball which pairs look more similar. You'll formalize this in the next exercise.

4. **Embedding dimensions** — Try a different model (e.g., `all-mpnet-base-v2`). How does the vector size change? Why might bigger vectors capture more nuance?

### What to look for
- Embeddings capture *semantic meaning*, not just word overlap.
- The same sentence always produces the same embedding (deterministic).
- Embedding models are separate from LLMs — they're smaller, faster, and purpose-built for this.
- This is what powers the "retrieval" in RAG: you embed the query, embed the documents, then find the closest matches.

---

## Exercise 3: `cosine_similarity.py`

**Goal:** Learn how to measure how "close" two embeddings are.

### Background
Cosine similarity measures the angle between two vectors, ignoring magnitude. It returns a value from -1 (opposite) to 1 (identical). It's the standard metric for comparing embeddings.

Formula: `cos(A, B) = (A · B) / (|A| × |B|)`

### Tasks

1. **Implement it yourself** — Write a function that computes cosine similarity from scratch using only basic Python (no libraries). This means:
   - Compute the dot product: `sum(a*b for a,b in zip(vec_a, vec_b))`
   - Compute magnitudes: `sum(x**2 for x in vec) ** 0.5`
   - Divide dot product by product of magnitudes

2. **Verify against sklearn** — Use `sklearn.metrics.pairwise.cosine_similarity` on the same vectors. Your results should match.

3. **Compare sentence pairs** — Using embeddings from Exercise 2, compute similarity for:
   - Semantically similar sentences (should be high, >0.7)
   - Semantically different sentences (should be low, <0.4)
   - Sentences that share words but differ in meaning (interesting middle ground)

4. **Build a mini search** — Create a list of 5-10 "documents" (just sentences). Embed them all. Then:
   - Take a query string, embed it
   - Compute cosine similarity against all documents
   - Rank by similarity and print the top 3
   - This is literally what a vector database does

### What to look for
- Cosine similarity ignores vector length — only direction matters. Two vectors of different magnitudes but same direction score 1.0.
- Your manual implementation should match sklearn's exactly (or very close due to floating point).
- The "mini search" you build here is a simplified version of what ChromaDB does in the RAG app.

---

## Exercise 4: `ner.py`

**Goal:** Extract structured entities (people, places, organizations, dates) from unstructured text.

### Background
Named Entity Recognition (NER) identifies and classifies named entities in text. It's a core NLP task used in information extraction, content analysis, and data pipelines. spaCy provides production-grade NER out of the box.

### Setup
```bash
pip install spacy
python -m spacy download en_core_web_sm
```

### Tasks

1. **Basic NER** — Load spaCy and process a text:
   ```python
   import spacy
   nlp = spacy.load("en_core_web_sm")
   doc = nlp("Apple is looking at buying U.K. startup for $1 billion")
   ```
   - Iterate over `doc.ents` and print each entity's `.text`, `.label_`, and `.start_char`/`.end_char`
   - Use `spacy.explain(label)` to understand what each label means (e.g., "ORG", "GPE", "MONEY")

2. **Try different texts** — Run NER on:
   - A news headline
   - A sentence about yourself (does it find your name?)
   - A technical sentence with product names
   - Notice where it gets things wrong — NER isn't perfect

3. **Entity frequency** — Process a longer text (a paragraph or two). Count entity types using a dict: how many ORGs, PERSONs, DATEs, etc.?

4. **Visualize** — Use `spacy.displacy.render(doc, style="ent")` to get an HTML visualization. Save it to a file and open it in a browser. This is useful for debugging and presentations.

5. **Compare models** — If time allows, try `en_core_web_md` or `en_core_web_lg`. Do they catch entities the small model misses?

### What to look for
- NER is a *classification* task at the token level — each token gets labeled.
- spaCy models are statistical, not rule-based. They can be wrong, especially on domain-specific text.
- Entity types are standardized (PERSON, ORG, GPE, DATE, MONEY, etc.) but coverage varies by model.
- In a real pipeline, you'd use NER to extract structured data from unstructured documents before feeding them to an LLM.

---

## Exercise 5: `text_classification.py`

**Goal:** Classify text into categories using a pretrained transformer model.

### Background
Text classification assigns a label to a piece of text — sentiment analysis ("positive"/"negative"), topic classification ("sports"/"politics"), intent detection ("question"/"command"). HuggingFace's `pipeline` API makes this easy with pretrained models.

### Tasks

1. **Sentiment analysis** — Use HuggingFace pipelines:
   ```python
   from transformers import pipeline
   classifier = pipeline("sentiment-analysis")
   ```
   - Classify several sentences. Print the label and confidence score.
   - Try ambiguous sentences — what does the model do with mixed sentiment?
   - Try sarcasm — does the model catch it?

2. **Zero-shot classification** — This is the powerful one:
   ```python
   classifier = pipeline("zero-shot-classification")
   result = classifier(
       "The new iPhone has an amazing camera but the battery life is disappointing",
       candidate_labels=["technology", "sports", "food", "politics"]
   )
   ```
   - You provide the categories at runtime — no training needed.
   - Try different label sets on the same text. Notice how scores shift.
   - Try labels that are close in meaning ("tech" vs "technology" vs "gadgets").

3. **Batch classification** — Classify a list of 5-10 texts at once. Organize results into a summary (e.g., "3 positive, 2 negative").

4. **Confidence thresholds** — Not all predictions are confident. Filter results by a confidence threshold (e.g., only keep predictions above 0.8). What percentage of your test cases survive?

### What to look for
- `pipeline()` hides a lot of complexity: tokenization, model inference, post-processing. Know that it's doing all three.
- Zero-shot classification uses a model trained on natural language inference (NLI) — it treats each label as a hypothesis and scores how well the text supports it.
- Confidence scores are model probabilities, not ground truth. A model can be confidently wrong.
- In production, you'd fine-tune a model on your specific labels rather than using zero-shot.

---

## Exercise 6: `nlp_unified_notebook.ipynb`

**Goal:** Tie everything together in a Jupyter notebook with explanations.

### Tasks

After completing exercises 1-5, create a notebook that:

1. **Walks through each concept** with markdown cells explaining what it is and why it matters
2. **Shows key code** from your scripts (don't just copy-paste everything — pick the most illustrative parts)
3. **Connects the dots** — explain how these concepts work together:
   - Tokenization → Embeddings (tokens are what get embedded)
   - Embeddings → Cosine Similarity (this is how you search)
   - Cosine Similarity → RAG (this is how retrieval works)
   - NER + Classification → preprocessing and routing in AI pipelines
4. **Include outputs** — run the cells so the notebook shows results inline

### What to look for
- The notebook should tell a story, not just dump code.
- A hiring manager scanning this should understand your grasp of how these pieces fit together.
- Keep it concise — this is a demo, not a textbook.

---

## Libraries You'll Need

```bash
pip install transformers sentence-transformers spacy scikit-learn torch jupyter
python -m spacy download en_core_web_sm
```

Save these to `requirements.txt` when you're ready.

## General Tips

- **Start each script with a docstring** explaining the concept and its relevance.
- **Print liberally.** Show shapes, types, and intermediate values so you can see what's happening.
- **Try wrong inputs.** What happens when you embed an empty string? Tokenize emoji? Run NER on code? Seeing failure modes builds intuition.
- **Keep scripts under 100 lines.** The notebook can be longer since it includes markdown.

When you're done, let me know and we'll move on to building the RAG app.
