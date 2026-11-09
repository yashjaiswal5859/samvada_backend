from app.api.websocket.twiml import router as twiml_router
from app.api.websocket.audio_stream import router as stream_router
from fastapi import APIRouter

websocket_router = APIRouter()
websocket_router.include_router(twiml_router)
websocket_router.include_router(stream_router)
