from pydantic import BaseModel
from typing import Optional

class EmotionEntry(BaseModel):
    timestamp: str
    emotion: str
    confidence: float

class EmotionState(BaseModel):
    timeline: list[EmotionEntry]
    previous_emotion: Optional[str]
    current_emotion: Optional[str]
    emotion_trend: Optional[str]
    emotion_velocity: Optional[float]
    emotion_confidence: Optional[float]
