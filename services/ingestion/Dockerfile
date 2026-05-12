FROM python:3.11-slim

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV FASTEMBED_CACHE_PATH=/app/.cache/fastembed

WORKDIR /app

# Install shared LLM package first (changes less frequently)
COPY shared/ /shared/
RUN pip install --no-cache-dir /shared

COPY ingestion/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Preload FastEmbed's sparse BM25 assets during the image build. In-cluster
# smoke tests should not depend on first-request DNS/network access to fetch
# model files.
RUN mkdir -p "$FASTEMBED_CACHE_PATH" \
    && python -c 'from fastembed import SparseTextEmbedding; list(SparseTextEmbedding(model_name="Qdrant/bm25").embed(["warm cache"]))' \
    && chmod -R a+rX /app/.cache

COPY shared/ ./shared/
COPY ingestion/app/ ./app/

RUN useradd --create-home appuser
USER appuser

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
