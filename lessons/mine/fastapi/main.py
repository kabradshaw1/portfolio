import fastapi

from fastapi import FastAPI

app = FastAPI(title="Ingestion API")

@app.get("/health")
async def health():
    return {"status": "ok"}

from fastapi.testclient import TestClient
client = TestClient(app)

response = client.get("/health")
print(f"Status: {response.status_code}")
print(f"Body: {response.json()}")