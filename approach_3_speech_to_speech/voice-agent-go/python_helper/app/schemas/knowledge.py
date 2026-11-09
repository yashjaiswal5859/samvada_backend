from pydantic import BaseModel
import uuid
from typing import Optional
from datetime import datetime

class KnowledgeUpload(BaseModel):
    # Form data typically handles the file, but we can represent metadata here
    source_type: str
    tenant_id: uuid.UUID

class KnowledgeSourceResponse(BaseModel):
    id: uuid.UUID
    filename: str
    source_type: str
    chunk_count: int
    status: str
    uploaded_at: datetime

    model_config = {"from_attributes": True}
