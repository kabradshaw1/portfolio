FROM python:3.11-slim

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1

WORKDIR /app

# Install shared package first (changes less frequently)
COPY shared/ /shared/
RUN pip install --no-cache-dir /shared

COPY ir-agent/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY shared/ ./shared/
COPY ir-agent/app/ ./app/
COPY ir-agent/fixtures/ ./fixtures/

RUN useradd --create-home appuser
USER appuser

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
