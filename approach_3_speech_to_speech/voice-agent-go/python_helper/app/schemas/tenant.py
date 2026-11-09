from pydantic import BaseModel
import uuid
from datetime import datetime
from typing import Optional

class TenantCreate(BaseModel):
    name: str
    plan: str
    config: dict = {}

class TenantResponse(BaseModel):
    id: uuid.UUID
    name: str
    plan: str
    config: dict
    created_at: datetime

    model_config = {"from_attributes": True}
