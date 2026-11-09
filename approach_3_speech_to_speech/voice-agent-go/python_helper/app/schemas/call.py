from pydantic import BaseModel
import uuid
from datetime import datetime
from typing import Optional, Any

class CallResponse(BaseModel):
    id: uuid.UUID
    session_id: str
    direction: Optional[str]
    status: Optional[str]
    outcome: Optional[str]
    started_at: Optional[datetime]
    ended_at: Optional[datetime]
    duration_sec: Optional[int]

    model_config = {"from_attributes": True}

class OutboundCallRequest(BaseModel):
    phone_number: str
    tenant_id: uuid.UUID

class BatchCampaignRequest(BaseModel):
    company_name: str
    stt_provider: Optional[str] = "deepgram"
    llm_model: Optional[str] = "gpt-4o-mini"
    tts_provider: Optional[str] = "openai"
    vad_provider: Optional[str] = "silero"
    audio_emotion_provider: Optional[str] = "emotion2vec"
    text_emotion_provider: Optional[str] = "distilroberta"

class WebSocketMessage(BaseModel):
    type: str
    data: Optional[Any] = None
    digit: Optional[str] = None
    text: Optional[str] = None
    is_final: Optional[bool] = None
    tool: Optional[str] = None
    result: Optional[dict] = None
    outcome: Optional[str] = None
