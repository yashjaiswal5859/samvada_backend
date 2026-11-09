from pydantic import BaseModel
from typing import Optional

class TTSStyle(BaseModel):
    tone: str
    pitch: str
    rate: str
    energy: str
    pause: str

class ResponsePlannerOutput(BaseModel):
    text: str
    tts_style: TTSStyle
