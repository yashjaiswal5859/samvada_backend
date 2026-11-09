from pydantic import BaseModel
from typing import Optional

class AnalyticsSummary(BaseModel):
    total_calls: int
    avg_duration_sec: float
    conversion_rate: float
    top_outcomes: dict[str, int]
    emotion_distribution: dict[str, float]
