from fastapi import FastAPI

app = FastAPI(title="Chat API")


@app.get("/health")
async def health():
    return {"status": "ok"}
