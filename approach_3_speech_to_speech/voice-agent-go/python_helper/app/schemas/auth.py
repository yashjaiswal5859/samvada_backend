from pydantic import BaseModel
from typing import Optional
import uuid
from datetime import datetime

class LoginRequest(BaseModel):
    email: str
    password: str

class TokenResponse(BaseModel):
    access_token: str
    token_type: str = "bearer"

class APIKeyCreate(BaseModel):
    name: str
    scopes: list[str] = []

class APIKeyResponse(BaseModel):
    id: uuid.UUID
    name: str
    key: Optional[str] = None  # Only returned on creation
    scopes: list[str]
    expires_at: Optional[datetime]
    created_at: datetime

    model_config = {"from_attributes": True}
