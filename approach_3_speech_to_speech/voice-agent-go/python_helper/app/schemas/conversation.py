from pydantic import BaseModel, Field
from typing import Optional
from datetime import datetime
from app.schemas.emotion import EmotionState
from app.schemas.strategy import StrategyOutput
from app.schemas.tool import ToolCallResult

class TranscriptEntry(BaseModel):
    role: str
    content: str
    timestamp: datetime
    is_final: bool

class ConversationState(BaseModel):
    session_id: str
    customer_phone: Optional[str] = None
    transcript: list[TranscriptEntry] = Field(default_factory=list)
    emotion_timeline: Optional[EmotionState] = None
    strategy: Optional[StrategyOutput] = None
    memory: dict = Field(default_factory=dict)
    intent_history: list[str] = Field(default_factory=list)
    tool_history: list[ToolCallResult] = Field(default_factory=list)
    turn_count: int = 0
