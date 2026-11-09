from pydantic import BaseModel
from typing import Optional

class ToolCallRequest(BaseModel):
    tool_name: str
    parameters: dict
    session_id: str

class ToolCallResult(BaseModel):
    tool_name: str
    status: str
    result: Optional[dict]
    duration_ms: int
