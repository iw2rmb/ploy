from fastapi import FastAPI
app = FastAPI()
@app.get("/healthz")
def healthz(): return "ok"
@app.get("/")
def root(): return "hello from python-apisvc"
if __name__ == "__main__":
    import uvicorn; uvicorn.run(app, host="0.0.0.0", port=8080)
