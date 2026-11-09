from pydantic import BaseModel
from typing import Optional

class CustomerState(BaseModel):
    emotion: str
    stress: float
    engagement: float
    trust: float
    patience: float
    urgency: float
    voice_stability: float
    speaking_rate_wpm: int
    interrupts_detected: bool
    silence_events: int

class ConversationStrategy(BaseModel):
    tone: str
    pace: str
    energy: str
    verbosity: str
    goal: str
    avoid_jokes: bool
    upsell_allowed: bool
    escalation_recommended: bool
    active_listening: bool
    acknowledge_emotion: bool

class StrategyOutput(BaseModel):
    customer_state: CustomerState
    conversation_strategy: ConversationStrategy
