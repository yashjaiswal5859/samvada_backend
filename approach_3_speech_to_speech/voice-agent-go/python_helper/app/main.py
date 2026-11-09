import asyncio
from fastapi import FastAPI
from contextlib import asynccontextmanager

from app.config import settings
from app.api.v1.router import api_router

@asynccontextmanager
async def lifespan(app: FastAPI):
    # Startup: Init DB, Redis, Kafka
    print("Starting up Voice Agent Platform...")
    yield
    # Shutdown: Close connections
    print("Shutting down Voice Agent Platform...")

app = FastAPI(
    title=settings.APP_NAME,
    version="0.1.0",
    lifespan=lifespan
)

app.include_router(api_router, prefix=settings.API_V1_PREFIX)

from app.api.websocket.router import websocket_router
app.include_router(websocket_router)

from app.api.v2.rag_session import router as rag_v2_router
app.include_router(rag_v2_router, prefix="/v2")

@app.get("/health")
async def health_check():
    return {"status": "ok"}
