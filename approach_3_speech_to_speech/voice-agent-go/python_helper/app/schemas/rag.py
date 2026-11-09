from pydantic import BaseModel
import uuid
from typing import Optional

class RAGQuery(BaseModel):
    query: str
    tenant_id: uuid.UUID
    top_k: int = 5
    filters: Optional[dict] = None

class RAGResult(BaseModel):
    text: str
    score: float
    source: str
    metadata: dict
